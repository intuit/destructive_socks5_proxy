/*
The MIT License (MIT)

Copyright (c) 2016 Intuit Inc.
*/

package destructive_socks5_proxy

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func hello(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello world!")
}

func FakeRequestWithConnectForProxy() (time.Duration, error) {
	fmt.Println("FakeRequestWithConnectForProxy")
	u, err := url.Parse("socks5://localhost:9000")
	if err != nil {
		fmt.Println("parse", err)
		return time.Now().Sub(time.Now()), err
	}

	dialer, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		fmt.Println("fromURL", err)
		return time.Now().Sub(time.Now()), err
	}
	conn, err := dialer.Dial("tcp", "localhost:8111")
	if err != nil {
		fmt.Println("Dial", err)
		return time.Now().Sub(time.Now()), err
	}
	st := time.Now()
	conn.Write([]byte("CONNECT localhost:8111 HTTP/1.1 \n"))
	conn.Write([]byte("dontcare\n"))
	data := make([]byte, 32*1024)
	_, err = conn.Read(data)
	fmt.Println(string(data))
	return time.Now().Sub(st), err
}

func SimpleClientRequest() (time.Duration, error) {
	fmt.Println("Simple")
	u, err := url.Parse("socks5://localhost:9000")
	if err != nil {
		fmt.Println("parse", err)
		return time.Now().Sub(time.Now()), err
	}

	dialer, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		fmt.Println("fromURL", err)
		return time.Now().Sub(time.Now()), err
	}

	transport := &http.Transport{Dial: dialer.Dial}
	client := http.Client{Transport: transport, Timeout: 10e9}
	st := time.Now()
	resp, err := client.Get("http://localhost:8111")
	if err != nil {
		return time.Now().Sub(st), err
	} else {
		defer resp.Body.Close()
		//		fmt.Println(resp.Header)
		data, err := ioutil.ReadAll(resp.Body)
		fmt.Println(len(data), err, resp.Status, time.Now().Sub(st))
	}
	return time.Now().Sub(st), err
}

func init() {
	go NewListenerForTcpCopyingProxy(":9000")
	http.HandleFunc("/", hello)
	go http.ListenAndServe(":8111", nil)
}

var short_duration = 1.001e9
var low_duration_range = 1e9
var high_duration_range = 1.2e9

func TestLatencyRemoteWrite(t *testing.T) {
	SetLatencyForHost("localhost", PER_REMOTE_WRITE, time.Duration(short_duration), 1)
	duration, err := FakeRequestWithConnectForProxy() //SimpleClientRequest()

	_, _, exists, err := GetLatencyForHost("localhost", PER_REMOTE_WRITE)
	if !exists {
		t.Error("localhost doesn't exist in map", err)
	}

	if err != nil {
		t.Error("got error", err)
	}
	if !(duration > time.Duration(low_duration_range) && duration < time.Duration(high_duration_range)) {
		t.Error("duration outside of expected range [1s,1.2s]", duration)
	}

	duration, err = FakeRequestWithConnectForProxy()
	if err != nil {
		t.Error("got error", err)
	}
	if !(duration > time.Duration(0) && duration < time.Duration(20e6)) {
		t.Error("duration outside of expected range [0,10ms]", duration)
	}
}

func TestLatencyRemoteRead(t *testing.T) {
	SetLatencyForHost("localhost", PER_REMOTE_READ, time.Duration(short_duration), 1)

	_, _, exists, err := GetLatencyForHost("localhost", PER_REMOTE_READ)
	if !exists {
		t.Error("localhost doesn't exist in map", err)
	}

	duration, err := FakeRequestWithConnectForProxy()
	if err != nil {
		t.Error("got error", err)
	}
	if !(duration > time.Duration(low_duration_range) && duration < time.Duration(high_duration_range)) {
		t.Error("duration outside of expected range [1s,1.2s]", duration)
	}

	duration, err = FakeRequestWithConnectForProxy()
	if err != nil {
		t.Error("got error", err)
	}
	if !(duration > time.Duration(0) && duration < time.Duration(10e6)) {
		t.Error("duration outside of expected range [0,10ms]", duration)
	}
}

func TestLatencyRemoteConnect(t *testing.T) {
	SetLatencyForHost("localhost", PER_REMOTE_CONNECT, time.Duration(short_duration), 1)

	_, _, exists, err := GetLatencyForHost("localhost", PER_REMOTE_CONNECT)
	if !exists {
		t.Error("localhost doesn't exist in map", err)
	}

	duration, err := FakeRequestWithConnectForProxy()
	if err != nil {
		t.Error("got error", err)
	}
	if !(duration > time.Duration(low_duration_range) && duration < time.Duration(high_duration_range)) {
		t.Error("duration outside of expected range [1s,1.2s]", duration)
	}

	duration, err = FakeRequestWithConnectForProxy()
	if err != nil {
		t.Error("got error", err)
	}
	if !(duration > time.Duration(0) && duration < time.Duration(10e6)) {
		t.Error("duration outside of expected range [0,10ms]", duration)
	}
}

func TestBlacklist(t *testing.T) {
	_, err := SetBlacklistForHost("localhost", true)
	if err == nil {
		t.Error("Expected an err but didn't get one")
	}
	Blacklist = true
	SetBlacklistForHost("localhost", true)

	_, err = SimpleClientRequest()
	if err == nil {
		t.Error("Expected an err but didn't get one")
	}

	SetBlacklistForHost("localhost", false)
	_, err = SimpleClientRequest()
	if err != nil {
		t.Error("got error", err)
	}

	Blacklist = false
}

func TestWhitelist(t *testing.T) {
	_, err := SetWhitelistForHost("google.com", true)
	if err == nil {
		t.Error("Expected an err but didn't get one")
	}

	Whitelist = true
	SetWhitelistForHost("localhost", true)
	fmt.Println(HostToAllow)
	_, err = SimpleClientRequest()
	if err != nil {
		t.Error("got error", err)
	}

	SetWhitelistForHost("localhost", false)
	_, err = SimpleClientRequest()
	if err == nil {
		t.Error("Expected an err but didn't get one")
	}

	Whitelist = false
}
