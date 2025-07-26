package main

import (
	"fmt"
	"net/url"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/samsaralc/kratos-transport/transport/websocket"
)

var testServer *websocket.Server

const (
	MessageTypeChat = iota + 1
)

type ChatMessage struct {
	Sender    string `json:"sender"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

func main() {
	wsSrv := websocket.NewServer(
		websocket.WithAddress(":8800"),
		websocket.WithPath("/ws"),
		websocket.WithSocketConnectHandler(handleConnect),
		websocket.WithCodec("json"),
	)

	testServer = wsSrv

	websocket.RegisterServerMessageHandler(wsSrv, MessageTypeChat, handleChatMessage)

	app := kratos.New(
		kratos.Name("websocket"),
		kratos.Server(
			wsSrv,
		),
	)
	if err := app.Run(); err != nil {
		log.Error(err)
	}
}

func handleConnect(sessionId websocket.SessionID, queries url.Values, connect bool) {
	if connect {
		fmt.Printf("[%s] connected [%+v]\n", sessionId, queries)
	} else {
		fmt.Printf("[%s] disconnect\n", sessionId)
	}
}

func handleChatMessage(sessionId websocket.SessionID, message *ChatMessage) error {
	fmt.Printf("[%s] Payload: %v\n", sessionId, message)

	testServer.Broadcast(MessageTypeChat, *message)

	return nil
}
