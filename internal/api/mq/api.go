package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mqctx "git.loyso.art/frx/mqplay/internal/ctx"

	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
)

type API struct {
	conn *amqp.Connection
	ch   *amqp.Channel

	logger      zerolog.Logger
	deliveryBus chan amqp.Delivery
}

func New(amqpURL string, logger zerolog.Logger) (*API, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to mq")
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, errors.Wrap(err, "unable to make a channel")
	}

	return &API{
		conn:        conn,
		ch:          ch,
		logger:      logger,
		deliveryBus: make(chan amqp.Delivery),
	}, nil
}

func (a *API) Run() error {
	logger := a.logger.With().Str("fn", "run").Logger()
	logger.Info().Msg("running loop")
	go func() {
		for {
			var d = <-a.deliveryBus
			var handleID = uuid.New()
			var ctx = mqctx.AppendID(context.Background(), handleID)
			var l = a.logger.With().Str("id", handleID).Str("exchange", d.Exchange).Str("type", d.Type).Logger()

			l.Debug().Msg("accepted")

			ctx = l.WithContext(ctx)

			err := handlers[d.Exchange+":fanout"](ctx, d)

			if err != nil {
				if !d.Redelivered {
					_ = d.Nack(false, true)
				} else {
					_ = d.Ack(false)
				}

				logger.Error().Err(err).Msg("unable to handle operation")
				continue
			}

			l.Debug().Msg("served")
			_ = d.Ack(false)
		}
	}()

	return a.initExchange()
}

func (a *API) initExchange() error {
	logger := a.logger.With().Str("fn", "init_exchange").Logger()
	logger.Info().Msg("preparing exchanges")
	for exchangetype, _ := range handlers {
		et := strings.Split(exchangetype, ":")
		exchange := et[0]
		etype := et[1]
		qName := fmt.Sprintf("%s-%d", exchange, time.Now().Unix())
		logger := logger.With().Str("exchange", exchange).Str("type", etype).Str("queue_name", qName).Logger()

		logger.Debug().Msg("declaring exchange")
		err := a.ch.ExchangeDeclare(exchange, etype, false, false, false, false, nil)
		if err != nil {
			return errors.Errorf("unable to declare exchange %s of type %s", exchange, etype)
		}

		logger.Debug().Msg("declaring queue")
		_, err = a.ch.QueueDeclare(qName, false, true, false, false, nil)
		if err != nil {
			return errors.Wrap(err, "unable to declare a queue")
		}

		logger.Debug().Msg("bingind to queue")
		err = a.ch.QueueBind(qName, "*:", exchange, false, nil)
		if err != nil {
			return errors.Wrap(err, "unable to bind to queue")
		}

		logger.Debug().Msg("beginning to consume from queue")
		c, err := a.ch.Consume(qName, qName+"1", false, false, false, false, nil)
		if err != nil {
			return errors.Wrap(err, "unable to consume from queue")
		}

		go func() {
			for d := range c {
				logger.Debug().Msg("incoming delivery")
				a.deliveryBus <- d
			}
		}()

		logger.Info().Msg("ready")
	}

	return nil
}

type handler func(context.Context, amqp.Delivery) error

var handlers = map[string]handler{
	"alarm:fanout": handleAlarm,
	"state:fanout": handleState,
}

type AlarmEvent struct {
	Name string
	When time.Time
}

func handleAlarm(ctx context.Context, d amqp.Delivery) error {
	logger := zerolog.Ctx(ctx)

	var event = AlarmEvent{}
	err := json.Unmarshal(d.Body, &event)
	if err != nil {
		return errors.Wrap(err, "unable to unmarshal data")
	}

	logger.Debug().Interface("alarm_event", event).Msg("alarm!!")
	return nil
}

type StateEvent struct {
	State     string
	CreatedAt time.Time
}

func handleState(ctx context.Context, d amqp.Delivery) error {
	logger := zerolog.Ctx(ctx)

	var event = StateEvent{}
	err := json.Unmarshal(d.Body, &event)
	if err != nil {
		return errors.Wrap(err, "unable to unmarshal data")
	}

	logger.Debug().Interface("state_event", event).Msg("new state")
	return nil
}
