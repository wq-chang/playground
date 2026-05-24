package app

import (
	"fmt"

	"go-services/backend/internal/config"
	"go-services/backend/internal/user"
	"go-services/library/kafka"
)

func newKafkaConsumer(cfg *config.Config, svc *service) (*kafka.Client, error) {
	kafkaClient, err := kafka.New(
		cfg.Kafka.BrokerURLs,
		cfg.Kafka.ConsumerGroupID,
		kafka.WithAuth(cfg.Kafka.Username, cfg.Kafka.Password, kafka.AuthMechanismScram512),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %w", err)
	}

	eventConsumer := user.NewEventConsumer(svc.UserEventCommandService)
	if err := kafkaClient.Consumer.AddTopic(cfg.Kafka.UserEventTopic, eventConsumer.HandleRecord); err != nil {
		kafkaClient.Close()
		return nil, fmt.Errorf("failed to register user event topic handler: %w", err)
	}

	return kafkaClient, nil
}
