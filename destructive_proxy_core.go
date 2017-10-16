/*
The MIT License (MIT)

Copyright (c) 2016 Intuit Inc.
*/

package destructive_socks5_proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/araddon/gou"
	"github.com/dchest/uniuri"
	"github.com/tawawhite/go-socks5"
)

//wraps locks around fn.
func read_locker(locker sync.RWMutex, fn func()) {
	locker.RLock()
	fn()
	locker.RUnlock()
}
func RW_Locker(locker sync.RWMutex, fn func()) {
	locker.Lock()
	fn()
	locker.Unlock()
}

//metrics
type Counter string

var (
	Counters     = make(map[string]float64)
	CountersSync sync.RWMutex
)

func (c Counter) Inc() { c.Add(1) }
func (c Counter) Dec() { c.Add(-1) }
func (c Counter) Add(v float64) {
	exists := false
	CountersSync.RLock()
	_, exists = Counters[string(c)]
	CountersSync.RUnlock()
	if !exists {
		CountersSync.Lock()
		Counters[string(c)] += 0
		CountersSync.Unlock()
	}
	CountersSync.Lock()
	Counters[string(c)] += v
	CountersSync.Unlock()

	// 	if _, exists := Counters[string(c)]; !exists {
	// 		Counters[string(c)] += 0
	// 	}
	// 	Counters[string(c)] += v
	// })
}

func init() {
	gou.SetupLogging("debug")
	gou.SetColorIfTerminal()
}

//core
const (
	PER_REMOTE_READ    = "per_remote_read"
	PER_REMOTE_WRITE   = "per_remote_write"
	PER_REMOTE_CONNECT = "per_remote_connect"
	ACTIVE_CONNS       = "conns;Active:All"
	TOTAL_CONNS        = "conns;Total;All"
	TOTAL_BYTES_IN     = "bytes;Total;In"
	TOTAL_BYTES_OUT    = "bytes;Total;Out"
)

type LatencyAndCountStruct struct {
	Latency time.Duration
	Count   int
}

//Destructive behaviors
var (
	HostToSleepPerRemoteWrite       = make(map[string]LatencyAndCountStruct)
	HostToSleepPerRemoteWriteSync   sync.RWMutex
	HostToSleepPerRemoteRead        = make(map[string]LatencyAndCountStruct)
	HostToSleepPerRemoteReadSync    sync.RWMutex
	HostToSleepPerRemoteConnect     = make(map[string]LatencyAndCountStruct)
	HostToSleepPerRemoteConnectSync sync.RWMutex
	Blacklist                       = false
	HostToClose                     = make(map[string]interface{})
	hostToCloseSync                 sync.RWMutex
	Whitelist                       = false
	HostToAllow                     = make(map[string]interface{})
	hostToAllowSync                 sync.RWMutex
)

func NewListenerForTcpCopyingProxy(addr string) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		gou.Error(err)
	}
	for {
		local, err := l.Accept()
		if err != nil {
			gou.Error(err)
			continue
		}
		go func() {
			rid := uniuri.NewLen(15)
			remote, remote_addr, err := socks5.HandleProtocol(local)
			if err != nil {
				gou.Error(err)
				return
			}

			gou.Infof("New connection. Address=%v; rid=%v;", *remote_addr, rid)

			Counter(fmt.Sprintf("conns;%v;Total", remote_addr.HostAndPort())).Inc()
			Counter(TOTAL_CONNS).Inc()

			Counter(ACTIVE_CONNS).Inc()
			defer Counter(ACTIVE_CONNS).Dec()

			//sleep if remote ip exists in hostToSleepPerRemoteConnect
			RW_Locker(HostToSleepPerRemoteConnectSync, func() {
				host := remote_addr.IP.String()
				latencyAndCount, exists := HostToSleepPerRemoteConnect[host]
				if !exists {
					if ip, err := socks5.ResolveToIpCaching(remote_addr.FQDN); err == nil {
						host = ip.String()
						latencyAndCount, exists = HostToSleepPerRemoteConnect[host]
					}
				}

				sleep := time.Duration(0)
				if exists && latencyAndCount.Latency > 0 && latencyAndCount.Count != 0 {
					sleep = latencyAndCount.Latency
					if latencyAndCount.Count >= 1 { //was explicitly set
						HostToSleepPerRemoteConnect[host] = LatencyAndCountStruct{latencyAndCount.Latency, latencyAndCount.Count - 1}
					}
				} else if latencyAndCount.Count == 0 { //was explicitly set and reached zero. Remove from map
					delete(HostToSleepPerRemoteConnect, host)
				} // latencyAndCount.count < 0 => continue to add latency

				if sleep > 0 {
					time.Sleep(sleep)
					gou.Infof("Slept per connect: %v; Address=%v; rid=%v;", sleep, *remote_addr, rid)
					Counter(fmt.Sprintf("latencyPerRequest;%v;Total", remote_addr.HostAndPort())).Add(sleep.Seconds())
				}
			})

			connDoneCh := make(chan interface{}, 2)

			go func(dst net.Conn, src net.Conn) {
				data := make([]byte, 32*1024)
				for {
					RW_Locker(HostToSleepPerRemoteReadSync, func() {
						host := remote_addr.IP.String()
						latencyAndCount, exists := HostToSleepPerRemoteRead[host]
						gou.Infof("remote_addr %v", remote_addr)
						if !exists {
							if ip, err := socks5.ResolveToIpCaching(remote_addr.FQDN); err == nil {
								host = ip.String()
								latencyAndCount, exists = HostToSleepPerRemoteRead[host]
							}
						}

						if !exists && remote_addr.ProxyHost != "" {
							if ip, err := socks5.ResolveToIpCaching(remote_addr.ProxyHost); err == nil {
								host = ip.String()
								latencyAndCount, exists = HostToSleepPerRemoteRead[host]
								//gou.Debugf("proxy ProxyHost=%v ip=%v latencyAndCount=%v exists=%v hostToSleepPerRemoteConnect=%v", remote_addr.ProxyHost, ip, latencyAndCount, exists, hostToSleepPerRemoteConnect)
							}

						}

						//gou.Infof("Here %v %v %v %v %v %v", hostToSleepPerRemoteRead, host, latencyAndCount.latency, latencyAndCount.count, exists, remote_addr)

						sleep := time.Duration(0)
						if exists && latencyAndCount.Latency > 0 && latencyAndCount.Count != 0 {
							sleep = latencyAndCount.Latency
							if latencyAndCount.Count >= 1 {
								gou.Infof("latencyAndCount.count > 1. %v", latencyAndCount.Count)
								HostToSleepPerRemoteRead[host] = LatencyAndCountStruct{latencyAndCount.Latency, latencyAndCount.Count - 1}
							} // latencyAndCount.count < 0 => continue to add latency
						} else if latencyAndCount.Count == 0 {
							delete(HostToSleepPerRemoteRead, host)
						}

						if sleep > 0 {
							time.Sleep(sleep)
							gou.Infof("Slept per remote read: %v; Address=%v; rid=%v;", sleep, *remote_addr, rid)
							Counter(fmt.Sprintf("latencyPerRemoteRead;%v;Total", remote_addr.HostAndPort())).Add(sleep.Seconds())
						}
					})
					n, err := src.Read(data)
					if err != nil {
						if err != io.EOF {
							gou.Error(err)
						}
						break
					}

					gou.Error(string(bytes.SplitN(data, []byte("\n"), 2)[0]))
					if bytes.HasPrefix(data, []byte("CONNECT")) {
						splt := bytes.SplitN(data, []byte("\n"), 2)
						splt = bytes.Split(splt[0], []byte(" "))
						splt = bytes.Split(splt[1], []byte(":"))
						remote_addr.ProxyHost = string(splt[0])
					}

					if Blacklist {
						closed := false
						RW_Locker(hostToCloseSync, func() {
							_, fqdn_exists := HostToClose[remote_addr.IP.String()]
							_, proxy_exists := HostToClose[remote_addr.ProxyHost]

							if fqdn_exists || proxy_exists {
								gou.Infof("Closing connection in blacklist. Address=%v; rid=%v;", *remote_addr, rid)
								Counter(fmt.Sprintf("closed;%v;Total", remote_addr.HostAndPort())).Inc()
								local.Close()
								remote.Close()
								closed = true
							} else {
								gou.Infof("Allowing connection not in blacklist. Address=%v; rid=%v;", *remote_addr, rid)
								Counter(fmt.Sprintf("allowed;%v;Total", remote_addr.HostAndPort())).Inc()
							}
						})
						if closed {
							break
						}
					}

					if Whitelist {
						closed := false
						RW_Locker(hostToAllowSync, func() {
							_, fqdn_exists := HostToAllow[remote_addr.IP.String()]
							_, proxy_exists := HostToAllow[remote_addr.ProxyHost]

							if !fqdn_exists || (remote_addr.ProxyHost != "" && !proxy_exists) {
								gou.Infof("Closing connection not in whitelist. Address=%v; rid=%v;", *remote_addr, rid)
								Counter(fmt.Sprintf("closed;%v;Total", remote_addr.HostAndPort())).Inc()
								local.Close()
								remote.Close()
								closed = true
							} else {
								gou.Infof("Allowing connection in whitelist. Address=%v; rid=%v;", *remote_addr, rid)
								Counter(fmt.Sprintf("allowed;%v;Total", remote_addr.HostAndPort())).Inc()
							}
						})
						if closed {
							break
						}
					}

					RW_Locker(HostToSleepPerRemoteWriteSync, func() {
						host := remote_addr.IP.String()
						latencyAndCount, exists := HostToSleepPerRemoteWrite[host]
						gou.Infof("remote_addr %v", remote_addr)
						if !exists {
							if ip, err := socks5.ResolveToIpCaching(remote_addr.FQDN); err == nil {
								host = ip.String()
								latencyAndCount, exists = HostToSleepPerRemoteWrite[host]
							}
						}

						if !exists && remote_addr.ProxyHost != "" {
							if ip, err := socks5.ResolveToIpCaching(remote_addr.ProxyHost); err == nil {
								host = ip.String()
								latencyAndCount, exists = HostToSleepPerRemoteWrite[host]
								gou.Debugf("proxy ProxyHost=%v ip=%v latencyAndCount=%v exists=%v hostToSleepPerRemoteConnect=%v", remote_addr.ProxyHost, ip, latencyAndCount, exists, HostToSleepPerRemoteConnect)
							}

						}

						//gou.Infof("Here %v %v %v %v %v %v", hostToSleepPerRemoteWrite, host, latencyAndCount.latency, latencyAndCount.count, exists, remote_addr)

						sleep := time.Duration(0)
						if exists && latencyAndCount.Latency > 0 && latencyAndCount.Count != 0 {
							sleep = latencyAndCount.Latency
							if latencyAndCount.Count >= 1 {
								gou.Infof("latencyAndCount.count > 1. %v", latencyAndCount.Count)
								HostToSleepPerRemoteWrite[host] = LatencyAndCountStruct{latencyAndCount.Latency, latencyAndCount.Count - 1}
							} // latencyAndCount.count < 0 => continue to add latency
						} else if latencyAndCount.Count == 0 {
							delete(HostToSleepPerRemoteWrite, host)
						}

						if sleep > 0 {
							time.Sleep(sleep)
							gou.Infof("Slept per remote write: %v; Address=%v; rid=%v;", sleep, *remote_addr, rid)
							Counter(fmt.Sprintf("latencyPerRemoteWrite;%v;Total", remote_addr.HostAndPort())).Add(sleep.Seconds())
						}
					})

					Counter(TOTAL_BYTES_OUT).Add(float64(n))
					Counter(fmt.Sprintf("bytes;%v;Out", remote_addr.HostAndPort())).Add(float64(n))
					Counter(fmt.Sprintf("writes;%v;Out", remote_addr.HostAndPort())).Inc()
					_, err = dst.Write(data[:n])
					if err != nil {
						gou.Error(err)
						break
					}

				}

				connDoneCh <- nil

			}(remote, local)

			go func(dst net.Conn, src net.Conn) {
				n, _ := io.Copy(dst, src) //Direct copy
				Counter(TOTAL_BYTES_IN).Add(float64(n))
				Counter(fmt.Sprintf("bytes;%v;In", remote_addr.HostAndPort())).Add(float64(n))
				Counter(fmt.Sprintf("writes;%v;In", remote_addr.HostAndPort())).Inc()
				connDoneCh <- nil
			}(local, remote)

			//Wait for one of the connections to complete
			<-connDoneCh

			time.Sleep(100e6) //allow any pending writes to complete
			local.Close()
			remote.Close()
			gou.Debugf("Closed connections for Address=%v; rid=%v;", remote_addr, rid)

		}()
	}
}

func SetLatencyForHost(host, _type string, latency time.Duration, count int) (string, error) {
	ip, err := socks5.ResolveToIpCaching(host)
	if err != nil {
		return "", err
	}

	_resolved_ip := ip.String()

	switch _type {
	case PER_REMOTE_WRITE:
		RW_Locker(HostToSleepPerRemoteWriteSync, func() {
			if latency > 0 {
				HostToSleepPerRemoteWrite[_resolved_ip] = LatencyAndCountStruct{latency, count}
			} else {
				delete(HostToSleepPerRemoteWrite, _resolved_ip)
			}
		})
	case PER_REMOTE_READ:
		RW_Locker(HostToSleepPerRemoteReadSync, func() {
			if latency > 0 {
				HostToSleepPerRemoteRead[_resolved_ip] = LatencyAndCountStruct{latency, count}
			} else {
				delete(HostToSleepPerRemoteRead, _resolved_ip)
			}
		})
	case PER_REMOTE_CONNECT:
		RW_Locker(HostToSleepPerRemoteConnectSync, func() {
			if latency > 0 {
				HostToSleepPerRemoteConnect[_resolved_ip] = LatencyAndCountStruct{latency, count}
			} else {
				delete(HostToSleepPerRemoteConnect, _resolved_ip)
			}
		})
	}

	gou.Infof("Set latency for %v (%v) to %v", host, ip, latency)
	return ip.String(), nil
}

func SetBlacklistForHost(host string, add bool) (string, error) {
	if !Blacklist {
		return "", fmt.Errorf("Blacklist is not set")
	}
	ip, err := socks5.ResolveToIpCaching(host)
	if err != nil {
		return "", err
	}

	_resolved_ip := ip.String()

	RW_Locker(hostToCloseSync, func() {
		if add {
			HostToClose[_resolved_ip] = add
		} else {
			delete(HostToClose, _resolved_ip)
		}
	})

	return ip.String(), nil
}

func SetWhitelistForHost(host string, add bool) (string, error) {
	if !Whitelist {
		return "", fmt.Errorf("Whitelist is not set")
	}

	ip, err := socks5.ResolveToIpCaching(host)
	if err != nil {
		return "", err
	}

	_resolved_ip := ip.String()

	RW_Locker(hostToAllowSync, func() {
		if add {
			HostToAllow[_resolved_ip] = add
		} else {
			delete(HostToAllow, _resolved_ip)
		}
	})

	return ip.String(), nil
}

func GetLatencyForHost(host, _type string) (string, LatencyAndCountStruct, bool, error) {
	ip, err := socks5.ResolveToIpCaching(host)
	if err != nil {
		return "", LatencyAndCountStruct{time.Duration(0), -1}, false, err
	}
	latencyAndCount, exists := LatencyAndCountStruct{time.Duration(0), -1}, false
	switch _type {
	case PER_REMOTE_WRITE:
		RW_Locker(HostToSleepPerRemoteWriteSync, func() {
			latencyAndCount, exists = HostToSleepPerRemoteWrite[ip.String()]
		})
	case PER_REMOTE_READ:
		RW_Locker(HostToSleepPerRemoteReadSync, func() {
			latencyAndCount, exists = HostToSleepPerRemoteRead[ip.String()]
		})
	case PER_REMOTE_CONNECT:
		RW_Locker(HostToSleepPerRemoteConnectSync, func() {
			latencyAndCount, exists = HostToSleepPerRemoteConnect[ip.String()]
		})
	}

	gou.Infof("Got latency for %v (%v) to %v. Exists=%v.", host, ip, latencyAndCount, exists)
	return ip.String(), latencyAndCount, exists, nil
}
