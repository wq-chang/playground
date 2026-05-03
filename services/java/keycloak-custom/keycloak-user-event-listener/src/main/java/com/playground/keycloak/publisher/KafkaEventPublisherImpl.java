package com.playground.keycloak.publisher;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.playground.keycloak.dto.EventMessage;
import java.time.Duration;
import java.util.concurrent.TimeUnit;
import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.clients.producer.RecordMetadata;
import org.jboss.logging.Logger;

public class KafkaEventPublisherImpl implements EventPublisher {

  private static final Logger logger = Logger.getLogger(KafkaEventPublisherImpl.class);
  private final KafkaProducer<String, byte[]> producer;
  private final ObjectMapper objectMapper;
  private final String topic;
  private final long sendTimeout;
  private final long closeTimeout;
  private final int maxRetries;
  private final long initialBackoffMillis;

  public KafkaEventPublisherImpl(
      KafkaProducer<String, byte[]> producer,
      ObjectMapper objectMapper,
      String topic,
      long sendTimeout,
      long closeTimeout,
      int maxRetries,
      long initialBackoffMillis) {
    this.producer = producer;
    this.objectMapper = objectMapper;
    this.topic = topic;
    this.sendTimeout = sendTimeout;
    this.closeTimeout = closeTimeout;
    this.maxRetries = maxRetries;
    this.initialBackoffMillis = initialBackoffMillis;
  }

  @Override
  public void publish(EventMessage message) {
    byte[] payload;

    try {
      payload = objectMapper.writeValueAsBytes(message);
    } catch (JsonProcessingException e) {
      logger.errorf(
          "Failed to serialize event for userId=%s: %s", message.userId(), e.getMessage());
      return;
    }

    long currentBackoff = initialBackoffMillis;

    for (int attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        ProducerRecord<String, byte[]> record =
            new ProducerRecord<>(topic, message.userId(), payload);
        RecordMetadata metadata = producer.send(record).get(sendTimeout, TimeUnit.MILLISECONDS);

        logger.infof(
            "Published to Kafka: topic=%s, partition=%d, offset=%d",
            metadata.topic(), metadata.partition(), metadata.offset());
        return;

      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return;
      } catch (Exception e) {
        logger.warnf(
            "Attempt %d/%d failed for topic=%s: %s", attempt, maxRetries, topic, e.getMessage());

        if (attempt == maxRetries) {
          handlePublishFailure(message, e);
          break;
        }

        try {
          Thread.sleep(currentBackoff);
        } catch (InterruptedException ie) {
          Thread.currentThread().interrupt();
          return;
        }
        currentBackoff *= 2;
      }
    }
  }

  private void handlePublishFailure(EventMessage message, Exception e) {
    // TODO: write to a file
  }

  @Override
  public void close() throws Exception {
    if (producer == null) {
      return;
    }

    producer.close(Duration.ofMillis(closeTimeout));
  }
}
