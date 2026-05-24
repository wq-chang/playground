package app

import (
	"fmt"

	"go-services/backend/internal/config"
	"go-services/backend/internal/user"
	"go-services/library/kafka"
)

func newKafkaConsumer(cfg *config.Config, svc *service) (*kafka.Consumer, error) {
	kafkaConsumer, _, err := kafka.New(
		cfg.Kafka.BrokerURLs,
		cfg.Kafka.ConsumerGroupID,
		kafka.WithAuth(cfg.Kafka.Username, cfg.Kafka.Password, kafka.AuthMechanismScram512),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %w", err)
	}

	eventConsumer := user.NewEventConsumer(svc.UserEventCommandService)
	if err := kafkaConsumer.AddTopic(cfg.Kafka.UserEventTopic, eventConsumer.HandleRecord); err != nil {
		kafkaConsumer.Close()
		return nil, fmt.Errorf("failed to register user event topic handler: %w", err)
	}

	return kafkaConsumer, nil
}
