# tcpSocket
A Lightweight Socket Service with heartbeat, Can be easily used in TCP server development.

Wiki Page [https://github.com/kisskamy/tcpSocket](https://github.com/kisskamy/tcpSocket)

## Requirements

Go version: 1.9.x or later

## Usage

```
go get -u github.com/9b9387/zero
```

```go
import "github.com/kisskamy/tcpSocket"

func main() {
 	host := "127.0.0.1:18787"

 	ss, err := tcpSocket.NewSocketService(host)
	if err != nil {
		return
	}

	// set Heartbeat
	ss.SetHeartBeat(5*time.Second, 30*time.Second)

	// net event
	ss.RegMessageHandler(HandleMessage)
	ss.RegConnectHandler(HandleConnect)
	ss.RegDisconnectHandler(HandleDisconnect)

	ss.Serv()
}
