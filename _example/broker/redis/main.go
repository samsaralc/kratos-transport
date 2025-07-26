package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samsaralc/kratos-transport/broker"
	"github.com/samsaralc/kratos-transport/broker/redis"
	api "github.com/samsaralc/kratos-transport/testing/api/manual"
)

const (
	localBroker = "127.0.0.1:6379"
	testTopic   = "test_topic"
)

func handleHygrothermograph(_ context.Context, topic string, headers broker.Headers, msg *api.Hygrothermograph) error {
	log.Infof("Topic %s, Headers: %+v, Payload: %+v\n", topic, headers, msg)
	return nil
}

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := redis.NewBroker(
		broker.WithCodec("json"),
		broker.WithAddress(localBroker),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		fmt.Println(err)
	}
	defer func(b broker.Broker) {
		err := b.Disconnect()
		if err != nil {
			fmt.Println(err)
		}
	}(b)

	_, _ = b.Subscribe(testTopic,
		api.RegisterHygrothermographJsonHandler(handleHygrothermograph),
		api.HygrothermographCreator,
	)

	<-interrupt
}
