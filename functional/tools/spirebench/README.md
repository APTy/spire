# spirebench

## Install
```
make functional/tools/spirebench
```

## Usage
```
$ ./functional/tools/spirebench/spirebench -help
Usage of ./functional/tools/spirebench/spirebench:
  -agent-addr string
        Location of SPIRE Agent Unix Listener. (default "unix:///tmp/agent.sock")
  -duration duration
        Benchmark duration. (default 10s)
  -rps int
        Target requests per second. (default 100)
  -server-addr string
        Location of SPIRE Server TCP Listener. (default "127.0.0.1:8081")
  -trust-domain string
        Trust domain of SPIRE Server. (default "example.org")
```
