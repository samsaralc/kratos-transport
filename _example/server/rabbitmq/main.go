package main

import (
	"context"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/samsaralc/kratos-transport/broker"
	rabbitmqBroker "github.com/samsaralc/kratos-transport/broker/rabbitmq"
	"github.com/samsaralc/kratos-transport/transport/rabbitmq"

	api "github.com/samsaralc/kratos-transport/testing/api/manual"
)

const (
	testBroker = "amqp://user:bitnami@127.0.0.1:5672"

	testExchange = "test_exchange"
	testQueue    = "test_queue"
	testRouting  = "test_routing_key"
)

func handleHygrothermograph(_ context.Context, topic string, headers broker.Headers, msg *api.Hygrothermograph) error {
	log.Infof("Topic %s, Headers: %+v, Payload: %+v\n", topic, headers, msg)
	return nil
}

func main() {
	ctx := context.Background()

	rabbitmqSrv := rabbitmq.NewServer(
		rabbitmq.WithAddress([]string{testBroker}),
		rabbitmq.WithCodec("json"),
		rabbitmq.WithExchange(testExchange, true),
	)

	_ = rabbitmq.RegisterSubscriber(rabbitmqSrv, ctx, testRouting,
		handleHygrothermograph,
		broker.WithQueueName(testQueue),
		rabbitmqBroker.WithDurableQueue())

	app := kratos.New(
		kratos.Name("rabbitmq"),
		kratos.Server(
			rabbitmqSrv,
		),
	)
	if err := app.Run(); err != nil {
		log.Error(err)
	}
}
