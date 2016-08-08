##Destructive Proxy

Utilizes the [Socks5 proxy protocol](https://www.ietf.org/rfc/rfc1928.txt) to proxy all socket traffic.

In addition to providing proxy functionality, the destructive proxy is able to inject latency around network calls and shutdown connections by hostname.


### Usage

##### Default
```bash
$PORT=4000 ./destructive_socks5_proxy
```
<br>
##### Commandline Options

#### Linux

```bash
$ ./destructive_socks5_proxy_linux_amd64 -help
Usage of ./destructive_socks5_proxy:
  -addr="0.0.0.0:9000": address to listen on
  -blacklist="": csv list of hosts to blacklist
  -config="todo": optional json config file of host:latency:type
  -whitelist="": csv list of hosts to whitelist.
```

#### Mac

```bash
$ ./destructive_socks5_proxy_darwin_amd64 -help
Usage of ./destructive_socks5_proxy:
  -addr="0.0.0.0:9000": address to listen on
  -blacklist="": csv list of hosts to blacklist
  -config="todo": optional json config file of host:latency:type
  -whitelist="": csv list of hosts to whitelist.
```


Note that the included binaries are located in the [destructive_socks5_proxy](destructive_socks5_proxy) subdirectory.

#### Java Parameters
* socksProxyHost
* socksProxyPort (default port = 1080)
* http://docs.oracle.com/javase/8/docs/technotes/guides/net/proxies.html

```bash
# For example:
java  -DsocksProxyHost=127.0.0.1 -DsocksProxyPort=9000 ...
```


### API


```bash
/set_latency/:host/per_remote_write?latency=100ms[&count=1]
```
* Set the per_remote_write latency for the :host parameter
	* Adds latency for every network write to the remote host. One connection can make many network writes, even for a single request. 
	* Optionally, specify a **count** value to limit the number of times the latency value is applied.
	  For instance, count=1 means the only 1 remote write will have the latency added. Note that count < 0, indicates means to continue to add latency to all remote writes until latency is explicitly removed. This is the default behavior.
* The latency parameter parses a duration string (e.g., 60000ms, 60s, 1m).  

```bash
/set_latency/:host/per_remote_read?latency=100ms[&count=1]
```
* Set the per_remote_read latency for the :host parameter
	* Adds latency for every network read from the remote host. One connection can make many network reads, even for a single request. 
	* Optionally, specify a **count** value to limit the number of times the latency value is applied.
	  For instance, count=1 means the only 1 remote read will have the latency added. Note that count < 0, indicates means to continue to add latency to all remote reads until latency is explicitly removed. This is the default behavior.
* The latency parameter parses a duration string (e.g., 60000ms, 60s, 1m).  

```bash
/set_latency/:host/per_remote_connect?latency=100ms[&count=1]
```
* Set the per_remote_connect latency for the :host parameter
	* Adds latency for each network connect to the remote host. Does not work very well for long lived connections (e.g., JMS, JDBC)
	* Optionally, specify a **count** value to limit the number of times the latency value is applied.
	  For instance, count=1 means the only 1 remote write will have the latency added. Note that count < 0, indicates means to continue to add latency to all remote writes until latency is explicitly removed. This is the default behavior.
	 
* The latency parameter parses a duration string (e.g., 60000ms, 60s, 1m).  


```bash
/get_latancy/:host/per_remote_write
```
* Lists hostname and latency for the :host parameter if latency has been set with per_remote_write parameter


```bash
/get_latancy/:host/per_remote_connect
```
* Lists hostname and latency for the :host parameter if latency has been set with per_remote_connect parameter


```bash
/get_latancy/all/per_remote_write
```
* Lists hostname and latency for all hosts that have had latency set using the per_remote_write parameter
 
```bash
/get_latancy/all/per_remote_connect
```
* Lists hostname and latency for all hosts that have had latency set using the per_remote_connect parameter

```bash
/counters
```
* Lists counts and metrics on which hosts have been seen, bytes, total latency, etc

```bash
/counters/reset
```
* Resets counters

```bash
/dependencies
```
* Lists dependencies

```bash
/whitelist/:host/:add_or_remove
```
* Add or remote a host to the whitelist.
  * Requires the server to be started with the -whitelist option.
  * Note: blacklist and whitelist can't be used at the same time.


```bash
/whitelisted
```
* Lists hosts that have been added to the whitelist


```bash
/blacklist/:host/:add_or_remove
```
* Add or remote a host to the blacklist.
  * Requires the server to be started with the -blacklist option.
  * Note: blacklist and whitelist can't be used at the same time.

```bash
/blacklisted
```
* Lists hosts that have been added to the whitelist
