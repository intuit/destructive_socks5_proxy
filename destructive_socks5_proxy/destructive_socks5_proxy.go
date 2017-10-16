/*
The MIT License (MIT)

Copyright (c) 2016 Intuit Inc.
*/

package main

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Unknwon/macaron"
	"github.com/araddon/gou"
	dsp "github.com/intuit/destructive_socks5_proxy"
)

//Allows us to short circuit requests when asserts call panic
func recover_asserts(ctx *macaron.Context) {
	if err := recover(); err != nil {
		ctx.Status(400)
		ctx.Write([]byte(fmt.Sprintf("%s", err)))

	}
}

func assert(condition bool, msg string) {
	if !condition {
		panic(msg)
	}
}
func assertErr(err error, msg string) {
	if err != nil {
		panic(fmt.Errorf("%s; err=%s", msg, err.Error()))
	}
}

func main() {

	wl := flag.String("whitelist", "", "csv list of hosts to whitelist.")
	bl := flag.String("blacklist", "", "csv list of hosts to blacklist")
	addr := flag.String("addr", "0.0.0.0:9000", "address to listen on")
	config := flag.String("config", "todo", "optional json config file of host:latency:type")

	flag.Parse()

	if *wl != "" && *bl != "" {
		fmt.Println("Can't set whitelist & blacklist")
		return
	}

	if *wl != "" {
		dsp.Whitelist = true
		for _, host := range strings.Split(*wl, ",") {
			dsp.SetWhitelistForHost(host, true)
		}
	}

	if *bl != "" {
		dsp.Blacklist = true
		for _, host := range strings.Split(*bl, ",") {
			dsp.SetWhitelistForHost(host, true)
		}
	}

	if *config != "todo" {
		fmt.Println("config isn't implemented yet")
	}

	go dsp.NewListenerForTcpCopyingProxy(*addr)

	app := macaron.Classic()
	app.Use(macaron.Renderer(macaron.RenderOptions{
		IndentJSON: true,
	}))
	app.Use(macaron.Recovery())

	set_latency := func(_type string) func(ctx *macaron.Context) {
		return func(ctx *macaron.Context) {
			defer recover_asserts(ctx)
			count := -1
			_latency := ctx.Req.URL.Query().Get("latency")
			_count := ctx.Req.URL.Query().Get("count")
			assert(_latency != "", "latency param not set")
			if _count != "" {
				if i, err := strconv.ParseInt(_count, 10, 64); err == nil {
					count = int(i)
				}

			}
			latency, err := time.ParseDuration(_latency)
			assertErr(err, "")

			host := ctx.Params("host")
			ip, err := dsp.SetLatencyForHost(host, _type, latency, count)
			assertErr(err, "")

			ctx.JSON(200, fmt.Sprintf("%v %v(%v) latency=%v. count=%v", _type, host, ip, latency.String(), count))
		}
	}

	get_latency := func(_type string) func(ctx *macaron.Context) {
		return func(ctx *macaron.Context) {
			host := ctx.Params("host")
			ip, latencyAndCount, exists, err := dsp.GetLatencyForHost(host, _type)
			assertErr(err, "")
			ctx.JSON(200, fmt.Sprintf("%v %v(%v) latency=%v. count=%v. found=%v.", _type, host, ip, latencyAndCount.Latency, latencyAndCount.Count, exists))
		}
	}

	app.Get("/whitelist/:host/:addorremove", func(ctx *macaron.Context) {
		host := ctx.Params("host")
		defer recover_asserts(ctx)

		add := ctx.Params("addorremove") == "add"
		ip, err := dsp.SetWhitelistForHost(host, add)
		assertErr(err, "")

		if add {
			ctx.JSON(200, fmt.Sprintf("Added to whitelist %v(%v).", host, ip))
		} else {
			ctx.JSON(200, fmt.Sprintf("Removed from whitelist %v(%v).", host, ip))
		}
	})
	app.Get("/whitelisted", func(ctx *macaron.Context) {
		var hosts []string
		for host, _ := range dsp.HostToAllow {
			hosts = append(hosts, host)
		}
		sort.Strings(hosts)
		ctx.JSON(200, hosts)
	})

	app.Get("/blacklist/:host/:addorremove", func(ctx *macaron.Context) {
		host := ctx.Params("host")
		defer recover_asserts(ctx)
		add := ctx.Params("add_or_remove") == "add"
		ip, err := dsp.SetBlacklistForHost(host, add)

		assertErr(err, "")

		if add {
			ctx.JSON(200, fmt.Sprintf("Added to blacklist %v(%v).", host, ip))
		} else {
			ctx.JSON(200, fmt.Sprintf("Removed from blacklist %v(%v).", host, ip))
		}
	})
	app.Get("/blacklisted", func(ctx *macaron.Context) {
		var hosts []string
		for host, _ := range dsp.HostToClose {
			hosts = append(hosts, host)
		}
		sort.Strings(hosts)
		ctx.JSON(200, hosts)
	})

	app.Get("/set_latency/:host/"+dsp.PER_REMOTE_WRITE, set_latency(dsp.PER_REMOTE_WRITE))
	app.Get("/set_latency/:host/"+dsp.PER_REMOTE_READ, set_latency(dsp.PER_REMOTE_READ))
	app.Get("/set_latency/:host/"+dsp.PER_REMOTE_CONNECT, set_latency(dsp.PER_REMOTE_CONNECT))

	app.Get("/get_latancy/all/"+dsp.PER_REMOTE_CONNECT, func(ctx *macaron.Context) {
		gou.Info(dsp.HostToSleepPerRemoteConnect)
		ctx.JSON(200, dsp.HostToSleepPerRemoteConnect)
	})
	app.Get("/get_latancy/all/"+dsp.PER_REMOTE_WRITE, func(ctx *macaron.Context) {
		gou.Info(dsp.HostToSleepPerRemoteWrite)
		ctx.JSON(200, dsp.HostToSleepPerRemoteWrite)
	})

	app.Get("/get_latancy/:host/"+dsp.PER_REMOTE_WRITE, get_latency(dsp.PER_REMOTE_WRITE))
	app.Get("/get_latancy/:host/"+dsp.PER_REMOTE_CONNECT, get_latency(dsp.PER_REMOTE_CONNECT))

	//metrics
	app.Get("/counters", func(ctx *macaron.Context) {
		var _counters map[string]float64
		dsp.RW_Locker(dsp.CountersSync, func() {
			_counters = dsp.Counters
		})
		ctx.JSON(200, _counters)
	})

	app.Get("/dependencies", func(ctx *macaron.Context) {
		var hosts = make([]string, 0)
		dsp.RW_Locker(dsp.CountersSync, func() {
			for key, _ := range dsp.Counters {
				if strings.Contains(key, "writes") && strings.Contains(key, "Out") {
					hosts = append(hosts, strings.Split(strings.Split(key, ";")[1], ":")[0])
				}
			}
		})
		ctx.JSON(200, hosts)
	})

	//metrics
	app.Get("/counters/reset", func(ctx *macaron.Context) {
		dsp.RW_Locker(dsp.CountersSync, func() {
			dsp.Counters = make(map[string]float64)
		})
		ctx.JSON(200, "reset counters")
	})
	app.Get("/", func(ctx *macaron.Context) {
		ctx.JSON(200, []string{
			"/whitelist/:host/:add_or_remove",
			"/blacklist/:host/:add_or_remove",
			"/whitelisted",
			"/blacklisted",
			"/set_latency/:host/" + dsp.PER_REMOTE_WRITE + "?latency=100ms[&count=1]",
			"/set_latency/:host/" + dsp.PER_REMOTE_READ + "?latency=100ms[&count=1]",
			"/set_latency/:host/" + dsp.PER_REMOTE_CONNECT + "?latency=100ms[&count=1]",
			"/get_latancy/:host/" + dsp.PER_REMOTE_WRITE,
			"/get_latancy/:host/" + dsp.PER_REMOTE_CONNECT,
			"/get_latancy/all/" + dsp.PER_REMOTE_WRITE,
			"/get_latancy/all/" + dsp.PER_REMOTE_CONNECT,
			"/counters",
			"/counters/reset",
			"/dependencies",
		})
	})

	app.Run()
}
