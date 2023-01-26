package main

import (
	"flag"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/websocket/v2"
)

const (
	localKeyBackendURL = "localKeyBackendURL"
	localKeyDataType   = "localKeyDataType"
	dataTypeText       = "text"
	dataTypeBinary     = "binary"
)

func main() {
	listenAddrPtr := flag.String("listen", ":6080", "listen address")
	backendAddrPtr := flag.String("backend", "", "backend addr")
	dataTypePtr := flag.String(
		"data",
		"text",
		"backend data type: text or binary",
	)
	flag.Parse()

	if backendAddrPtr == nil || *backendAddrPtr == "" {
		log.Fatalln("Missing backend parameter. Use -h to help")
	}
	if *dataTypePtr != dataTypeText && *dataTypePtr != dataTypeBinary {
		log.Fatalln("Unsupported value for data parameter. Use -h to help")
	}

	log.Println("* Listen on:", *listenAddrPtr)
	log.Println("* Proxy to backend:", *backendAddrPtr)
	log.Println("* Backend data type:", *dataTypePtr)

	app := fiber.New(fiber.Config{
		Immutable: true,
	})

	app.Use(logger.New())
	app.Get(
		"/",
		wsCheckMiddleware(*backendAddrPtr, *dataTypePtr),
		websocket.New(wsHandler),
	)
	app.Listen(*listenAddrPtr)
}

func wsCheckMiddleware(backendURL string, dataType string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		c.Locals(localKeyBackendURL, backendURL)
		c.Locals(localKeyDataType, dataType)
		return c.Next()
	}
}

func wsHandler(c *websocket.Conn) {
	clientID := strconv.FormatUint(uint64(time.Now().UnixMicro()), 36)
	defer func() {
		c.Close()
		log.Println("=\\= client", clientID, "disconnected")
	}()

	log.Println("==> client", clientID, "connected")
	url := c.Locals(localKeyBackendURL).(string)

	udpServer, err := net.ResolveUDPAddr("udp", url)
	if err != nil {
		log.Fatalln(err)
	}
	udpConn, err := net.DialUDP("udp", nil, udpServer)
	if err != nil {
		log.Fatalln(err)
	}
	defer udpConn.Close()

	clientErrChan := make(chan error, 1)
	backendErrChan := make(chan error, 1)

	go forwardWS2UDP(c, udpConn, clientErrChan)
	go forwardUDP2WS(udpConn, c, backendErrChan)

	var msg string

	select {
	case err = <-clientErrChan:
		msg = "forward client to backend server error"
	case err = <-backendErrChan:
		msg = "forward backend to client server error"
	}

	if websocket.IsUnexpectedCloseError(
		err,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived) {
		log.Println(msg, "error:", err)
	}
}

func forwardWS2UDP(
	wsConn *websocket.Conn,
	udpConn *net.UDPConn,
	errChan chan error,
) {
	for {
		_, msg, err := wsConn.ReadMessage()
		if err != nil {
			errChan <- err
			break
		}

		_, err = udpConn.Write(msg)
		if err != nil {
			errChan <- err
			break
		}
	}
}

func forwardUDP2WS(
	udpConn *net.UDPConn,
	wsConn *websocket.Conn,
	errChan chan error,
) {
	dataType := wsConn.Locals(localKeyDataType).(string)
	wsMsgType := websocket.TextMessage
	if dataType == dataTypeBinary {
		wsMsgType = websocket.BinaryMessage
	}

	buf := make([]byte, 1024)
	for {
		n, err := udpConn.Read(buf)
		if err != nil {
			errChan <- err
			break
		}

		err = wsConn.WriteMessage(wsMsgType, buf[:n])
		if err != nil {
			errChan <- err
			break
		}
	}
}
