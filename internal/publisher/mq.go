package publisher

import (
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
)

type MQ struct {
	amqpURL  string
	readyc   chan struct{}
	errc     chan error
	firstRun bool
	logger   zerolog.Logger
	conn     *amqp.Connection
	ch       *amqp.Channel
	atype    MQType

	exchanges map[string]Exchange
}

type MQType uint8

const (
	Unknown MQType = iota
	// Publisher serves publish methods
	Publisher
	// Consumer consumes messages
	Consumer
)

// New opens connection to RabbitMQ but do not inits exchange
func New(amqpURL string, l zerolog.Logger, atype MQType) (*MQ, error) {
	logger := l.With().Str("pkg", "mq").Logger()

	if atype == Unknown {
		atype = Publisher
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to amqp")
	}

	s := &MQ{
		amqpURL:   amqpURL,
		conn:      conn,
		errc:      make(chan error),
		readyc:    make(chan struct{}),
		firstRun:  true,
		logger:    logger,
		atype:     atype,
		exchanges: map[string]Exchange{},
	}

	s.runWatchers()
	return s, nil
}

func (s *MQ) runWatchers() {
	go s.watchErrors()
	go s.watchConnection()
	go s.watchChannel()
}

func (s *MQ) watchErrors() {
	logger := s.logger.With().Str("fn", "watch_errors").Logger()

	var errChan error
	for errChan = range s.errc {
		logger.Error().Err(errChan).Msg("caught an error")
	}
}

func (s *MQ) watchConnection() {
	logger := s.logger.With().Str("fn", "watchConnection").Logger()
	logger.Info().Msg("watching for connection")
	var errClose error
	var closeNotify = make(chan *amqp.Error)

	for {
		if s.conn == nil {
			s.logger.Info().Msg("connecting to mq")
			s.conn, errClose = amqp.Dial(s.amqpURL)
			if errClose != nil {
				s.errc <- errClose
				continue
			}

			s.conn.NotifyClose(closeNotify)
		}

		errClose = <-closeNotify
		s.errc <- errClose
		s.conn = nil
		logger.Info().Msg("reconnecting in 5 seconds")
		time.Sleep(time.Second * 5)
	}
}

func (s *MQ) watchChannel() {
	logger := s.logger.With().Str("fn", "watch_channel").Logger()
	logger.Info().Msg("watching for channel")

	var errClose error
	var closeNotify = make(chan *amqp.Error)

	for {
		if s.conn == nil {
			logger.Info().Msg("not connected to rabbit, sleeping 10 seconds")
			time.Sleep(time.Second * 10)
			continue
		}

		if s.ch == nil {
			s.ch, errClose = s.conn.Channel()
			if errClose != nil {
				s.errc <- errClose
				continue
			}

			s.conn.NotifyClose(closeNotify)
			s.reinitExchanges()
			if s.firstRun {
				s.readyc <- struct{}{}
				s.firstRun = false
			}
		}

		errClose = <-closeNotify
		s.errc <- errClose
		s.ch = nil
		logger.Info().Msg("reconnecting in 10 seconds")
		time.Sleep(time.Second * 10)
	}
}

func (s *MQ) reinitExchanges() {
	if s.ch == nil {
		return
	}

	for _, v := range s.exchanges {
		err := s.ch.ExchangeDeclare(v.Name, v.Etype, false, false, false, false, nil)
		if err != nil {
			s.errc <- err
		}

		if s.atype == Publisher {
			continue
		}
	}
}

// Ready waits for signal
func (s *MQ) Ready() {
	<-s.readyc
}

// ConnectToExchange declares new exchange and prepares to publish messages
func (s *MQ) ConnectToExchange(name, etype string) error {
	if s.conn == nil {
		return errors.New("not connected to mq")
	}

	if s.ch == nil {
		return errors.New("channel is closed")
	}

	if _, ok := s.exchanges[name]; ok {
		// already have this exchange, exiting
		return nil
	}

	err := s.ch.ExchangeDeclare(name, etype, false, false, false, false, nil)
	if err != nil {
		return errors.Wrap(err, "unable to declare exchange")
	}

	s.exchanges[name] = Exchange{Name: name, Etype: etype}
	return nil
}

// Send publishes message to exchange
func (s *MQ) Send(name string, data []byte, routingKey string) error {
	if s.atype == Consumer {
		return errors.New("mq client is not for sending")
	}

	if _, ok := s.exchanges[name]; !ok {
		return errors.New("exchange has not been created")
	}
	msg := amqp.Publishing{
		ContentType: "application/json",
		Body:        data,
	}

	return s.ch.Publish(name, routingKey, false, false, msg)
}
