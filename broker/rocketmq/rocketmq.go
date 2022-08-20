package rocketmq

import (
	"context"
	"errors"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semConv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-transport/broker"
)

const (
	defaultAddr = "127.0.0.1:9876"
)

type rocketmqBroker struct {
	nameServers   []string
	nameServerUrl string

	accessKey    string
	secretKey    string
	instanceName string
	groupName    string
	retryCount   int
	namespace    string

	enableTrace bool

	connected bool
	sync.RWMutex
	opts broker.Options

	producers map[string]rocketmq.Producer
}

func NewBroker(opts ...broker.Option) broker.Broker {
	options := broker.NewOptionsAndApply(opts...)

	if v, ok := options.Context.Value(enableAliyunHttpKey{}).(bool); ok && v {
		return newAliyunHttpBroker(options)
	} else {
		return newBroker(options)
	}
}

func newBroker(options broker.Options) broker.Broker {
	return &rocketmqBroker{
		producers:  make(map[string]rocketmq.Producer),
		opts:       options,
		retryCount: 2,
	}
}

func (r *rocketmqBroker) Name() string {
	return "rocketmq"
}

func (r *rocketmqBroker) Address() string {
	if len(r.nameServers) > 0 {
		return r.nameServers[0]
	} else if r.nameServerUrl != "" {
		return r.nameServerUrl
	}
	return defaultAddr
}

func (r *rocketmqBroker) Options() broker.Options {
	return r.opts
}

func (r *rocketmqBroker) Init(opts ...broker.Option) error {
	r.opts.Apply(opts...)

	if v, ok := r.opts.Context.Value(nameServersKey{}).([]string); ok {
		r.nameServers = v
	}
	if v, ok := r.opts.Context.Value(nameServerUrlKey{}).(string); ok {
		r.nameServerUrl = v
	}
	if v, ok := r.opts.Context.Value(accessKey{}).(string); ok {
		r.accessKey = v
	}
	if v, ok := r.opts.Context.Value(secretKey{}).(string); ok {
		r.secretKey = v
	}
	if v, ok := r.opts.Context.Value(retryCountKey{}).(int); ok {
		r.retryCount = v
	}
	if v, ok := r.opts.Context.Value(namespaceKey{}).(string); ok {
		r.namespace = v
	}
	if v, ok := r.opts.Context.Value(instanceNameKey{}).(string); ok {
		r.instanceName = v
	}
	if v, ok := r.opts.Context.Value(groupNameKey{}).(string); ok {
		r.groupName = v
	}
	if v, ok := r.opts.Context.Value(enableTraceKey{}).(bool); ok {
		r.enableTrace = v
	}

	return nil
}

func (r *rocketmqBroker) Connect() error {
	r.RLock()
	if r.connected {
		r.RUnlock()
		return nil
	}
	r.RUnlock()

	p, err := r.createProducer()
	if err != nil {
		return err
	}

	_ = p.Shutdown()

	r.Lock()
	r.connected = true
	r.Unlock()

	return nil
}

func (r *rocketmqBroker) Disconnect() error {
	r.RLock()
	if !r.connected {
		r.RUnlock()
		return nil
	}
	r.RUnlock()

	r.Lock()
	defer r.Unlock()
	for _, p := range r.producers {
		if err := p.Shutdown(); err != nil {
			return err
		}
	}

	r.connected = false
	return nil
}

func (r *rocketmqBroker) createNsResolver() primitive.NsResolver {
	if len(r.nameServers) > 0 {
		return primitive.NewPassthroughResolver(r.nameServers)
	} else if r.nameServerUrl != "" {
		return primitive.NewHttpResolver("DEFAULT", r.nameServerUrl)
	} else {
		return primitive.NewHttpResolver("DEFAULT", defaultAddr)
	}
}

func (r *rocketmqBroker) createProducer() (rocketmq.Producer, error) {
	credentials := primitive.Credentials{
		AccessKey: r.accessKey,
		SecretKey: r.secretKey,
	}

	resolver := r.createNsResolver()

	var traceCfg *primitive.TraceConfig = nil
	if r.enableTrace {
		traceCfg = &primitive.TraceConfig{
			GroupName:   r.groupName,
			Credentials: credentials,
			Access:      primitive.Cloud,
			Resolver:    resolver,
		}
	}

	p, err := rocketmq.NewProducer(
		producer.WithNsResolver(resolver),
		producer.WithCredentials(credentials),
		producer.WithTrace(traceCfg),
		producer.WithRetry(r.retryCount),
		producer.WithInstanceName(r.instanceName),
		producer.WithNamespace(r.namespace),
		producer.WithGroupName(r.groupName),
	)
	if err != nil {
		log.Errorf("[rocketmq]: new producer error: " + err.Error())
		return nil, err
	}

	err = p.Start()
	if err != nil {
		log.Errorf("[rocketmq]: start producer error: %s", err.Error())
		return nil, err
	}

	return p, nil
}

func (r *rocketmqBroker) createConsumer(options *broker.SubscribeOptions) (rocketmq.PushConsumer, error) {
	credentials := primitive.Credentials{
		AccessKey: r.accessKey,
		SecretKey: r.secretKey,
	}

	resolver := r.createNsResolver()

	var traceCfg *primitive.TraceConfig = nil
	if r.enableTrace {
		traceCfg = &primitive.TraceConfig{
			GroupName:   options.Queue,
			Credentials: credentials,
			Access:      primitive.Cloud,
			Resolver:    resolver,
		}
	}

	c, _ := rocketmq.NewPushConsumer(
		consumer.WithNsResolver(resolver),
		consumer.WithCredentials(credentials),
		consumer.WithTrace(traceCfg),
		consumer.WithGroupName(options.Queue),
		consumer.WithAutoCommit(options.AutoAck),
		consumer.WithRetry(r.retryCount),
		consumer.WithNamespace(r.namespace),
		consumer.WithInstance(r.instanceName),
	)

	if c == nil {
		return nil, errors.New("create consumer error")
	}

	return c, nil
}

func (r *rocketmqBroker) Publish(topic string, msg broker.Any, opts ...broker.PublishOption) error {
	buf, err := broker.Marshal(r.opts.Codec, msg)
	if err != nil {
		return err
	}

	return r.publish(topic, buf, opts...)
}

func (r *rocketmqBroker) publish(topic string, msg []byte, opts ...broker.PublishOption) error {
	options := broker.PublishOptions{
		Context: context.Background(),
	}
	for _, o := range opts {
		o(&options)
	}

	var cached bool

	r.Lock()
	p, ok := r.producers[topic]
	if !ok {
		var err error
		p, err = r.createProducer()
		if err != nil {
			r.Unlock()
			return err
		}

		r.producers[topic] = p
	} else {
		cached = true
	}
	r.Unlock()

	rMsg := primitive.NewMessage(topic, msg)

	if v, ok := options.Context.Value(compressKey{}).(bool); ok {
		rMsg.Compress = v
	}
	if v, ok := options.Context.Value(batchKey{}).(bool); ok {
		rMsg.Batch = v
	}
	if v, ok := options.Context.Value(propertiesKey{}).(map[string]string); ok {
		rMsg.WithProperties(v)
	}
	if v, ok := options.Context.Value(delayTimeLevelKey{}).(int); ok {
		rMsg.WithDelayTimeLevel(v)
	}
	if v, ok := options.Context.Value(tagsKey{}).(string); ok {
		rMsg.WithTag(v)
	}
	if v, ok := options.Context.Value(keysKey{}).([]string); ok {
		rMsg.WithKeys(v)
	}
	if v, ok := options.Context.Value(shardingKeyKey{}).(string); ok {
		rMsg.WithShardingKey(v)
	}

	span := r.startProducerSpan(rMsg)

	var err error
	var ret *primitive.SendResult
	ret, err = p.SendSync(r.opts.Context, rMsg)
	if err != nil {
		log.Errorf("[rocketmq]: send message error: %s\n", err)
		switch cached {
		case false:
		case true:
			r.Lock()
			if err = p.Shutdown(); err != nil {
				r.Unlock()
				break
			}
			delete(r.producers, topic)
			r.Unlock()

			p, err = r.createProducer()
			if err != nil {
				r.Unlock()
				break
			}
			if ret, err = p.SendSync(r.opts.Context, rMsg); err == nil {
				r.Lock()
				r.producers[topic] = p
				r.Unlock()
				break
			}
		}
	}

	var messageId string
	if ret != nil {
		messageId = ret.MsgID
	}

	r.finishProducerSpan(span, messageId, err)

	return err
}

func (r *rocketmqBroker) Subscribe(topic string, handler broker.Handler, binder broker.Binder, opts ...broker.SubscribeOption) (broker.Subscriber, error) {
	options := broker.SubscribeOptions{
		Context: context.Background(),
		AutoAck: true,
		Queue:   r.groupName,
	}
	for _, o := range opts {
		o(&options)
	}

	c, err := r.createConsumer(&options)
	if err != nil {
		return nil, err
	}

	sub := &subscriber{
		opts:    options,
		topic:   topic,
		handler: handler,
		reader:  c,
	}

	if err := c.Subscribe(topic, consumer.MessageSelector{},
		func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
			//log.Infof("[rocketmq] subscribe callback: %v \n", msgs)

			var err error
			var m broker.Message
			for _, msg := range msgs {
				p := &publication{topic: msg.Topic, reader: sub.reader, m: &m, rm: &msg.Message, ctx: options.Context}

				span := r.startConsumerSpan(msg)

				m.Headers = msg.GetProperties()

				if binder != nil {
					m.Body = binder()
				}

				if err := broker.Unmarshal(r.opts.Codec, msg.Body, m.Body); err != nil {
					p.err = err
					log.Error(err)
				}

				err = sub.handler(sub.opts.Context, p)
				if err != nil {
					log.Errorf("[rocketmq]: process message failed: %v", err)
				}
				if sub.opts.AutoAck {
					if err = p.Ack(); err != nil {
						log.Errorf("[rocketmq]: unable to commit msg: %v", err)
					}
				}

				r.finishConsumerSpan(span)
			}

			return consumer.ConsumeSuccess, nil
		}); err != nil {
		log.Errorf(err.Error())
		return nil, err
	}

	if err := c.Start(); err != nil {
		log.Errorf(err.Error())
		return nil, err
	}

	return sub, nil
}

func (r *rocketmqBroker) startProducerSpan(msg *primitive.Message) trace.Span {
	if r.opts.Tracer.Tracer == nil {
		return nil
	}

	carrier := NewProducerMessageCarrier(msg)
	ctx := r.opts.Tracer.Propagators.Extract(r.opts.Context, carrier)

	attrs := []attribute.KeyValue{
		semConv.MessagingSystemKey.String("rocketmq"),
		semConv.MessagingDestinationKindTopic,
		semConv.MessagingDestinationKey.String(msg.Topic),
	}
	opts := []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindProducer),
	}
	ctx, span := r.opts.Tracer.Tracer.Start(ctx, "rocketmq.produce", opts...)

	r.opts.Tracer.Propagators.Inject(ctx, carrier)

	return span
}

func (r *rocketmqBroker) finishProducerSpan(span trace.Span, messageId string, err error) {
	if span == nil {
		return
	}

	span.SetAttributes(
		semConv.MessagingMessageIDKey.String(messageId),
		semConv.MessagingRocketmqNamespaceKey.String(r.namespace),
		semConv.MessagingRocketmqClientGroupKey.String(r.groupName),
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}

	span.End()
}

func (r *rocketmqBroker) startConsumerSpan(msg *primitive.MessageExt) trace.Span {
	if r.opts.Tracer.Tracer == nil {
		return nil
	}

	carrier := NewConsumerMessageCarrier(msg)
	ctx := r.opts.Tracer.Propagators.Extract(r.opts.Context, carrier)

	attrs := []attribute.KeyValue{
		semConv.MessagingSystemKey.String("rocketmq"),
		semConv.MessagingDestinationKindTopic,
		semConv.MessagingDestinationKey.String(msg.Topic),
		semConv.MessagingOperationReceive,
		semConv.MessagingMessageIDKey.String(msg.MsgId),
	}
	opts := []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindConsumer),
	}
	newCtx, span := r.opts.Tracer.Tracer.Start(ctx, "rocketmq.consume", opts...)

	r.opts.Tracer.Propagators.Inject(newCtx, carrier)

	return span
}

func (r *rocketmqBroker) finishConsumerSpan(span trace.Span) {
	if span == nil {
		return
	}

	span.End()
}
