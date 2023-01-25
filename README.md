# udpwsproxy
A toy to proxy udp server data to websocket server

## Usage

```bash
$ go run main.go -listen 127.0.0.1:10001 -backend 127.0.0.1:1053 -data text
```

for more, use:
```bash
$ go run main.go -h
```
