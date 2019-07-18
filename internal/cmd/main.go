package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"git.loyso.art/frx/mqplay/internal/api/mq"
	"git.loyso.art/frx/mqplay/internal/publisher"
	"github.com/rs/zerolog"
)

var (
	revision  string = "unknown"
	buildtime string = "unknown"
)

const amqpURL = "amqp://192.168.99.100:5672"

func main() {
	log.Printf("rev: %s\tbt: %s", revision, buildtime)

	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	api, err := mq.New(amqpURL, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to init api")
	}

	err = api.Run()
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to run api")
	}

	p, err := publisher.New(amqpURL, logger, publisher.Publisher)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to init publisher")
	}

	p.Ready()

	if err := p.ConnectToExchange("state", "fanout"); err != nil {
		logger.Fatal().Err(err).Msg("error found")
	}
	if err := p.ConnectToExchange("alarm", "fanout"); err != nil {
		logger.Fatal().Err(err).Msg("error found")
	}

	for {
		fmt.Print("type message: ")
		var msg string
		_, err = fmt.Scanln(&msg)
		if err != nil {
			panic(err)
		}

		switch msg {
		case "alarm":
			d, _ := json.Marshal(&mq.AlarmEvent{
				Name: "oh well",
				When: time.Now(),
			})
			err = p.Send("alarm", d, "*")
			if err != nil {
				panic(err)
			}
		case "state":
			d, _ := json.Marshal(&mq.StateEvent{
				State:     "sleeping",
				CreatedAt: time.Now(),
			})
			err = p.Send("state", d, "*")
			if err != nil {
				panic(err)
			}
		case "quit", "q":
			os.Exit(0)
		default:
			continue
		}
	}

}
