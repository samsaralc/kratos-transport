package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/samsaralc/kratos-transport/broker"
	"github.com/samsaralc/kratos-transport/broker/rabbitmq"
	api "github.com/samsaralc/kratos-transport/testing/api/manual"
)

const (
	testBroker = "amqp://user:bitnami@127.0.0.1:5672"

	testExchange = "test_exchange"
	testQueue    = "test_queue"
	testRouting  = "test_routing_key"
)

func handleHygrothermograph(_ context.Context, topic string, headers broker.Headers, msg *api.Hygrothermograph) error {
	fmt.Printf("Topic %s, Headers: %+v, Payload: %+v\n", topic, headers, msg)
	return nil
}

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := rabbitmq.NewBroker(
		broker.WithCodec("json"),
		broker.WithAddress(testBroker),
		rabbitmq.WithExchangeName(testExchange),
		rabbitmq.WithDurableExchange(),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		fmt.Println(err)
	}
	defer b.Disconnect()

	_, _ = b.Subscribe(testRouting,
		api.RegisterHygrothermographJsonHandler(handleHygrothermograph),
		api.HygrothermographCreator,
		broker.WithQueueName(testQueue),
		// broker.WithDisableAutoAck(),
		rabbitmq.WithDurableQueue(),
	)

	<-interrupt
}
