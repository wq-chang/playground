package com.playground.keycloak.publisher;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.playground.keycloak.dto.EventMessage;
import io.nats.client.JetStream;
import io.nats.client.JetStreamApiException;
import io.nats.client.api.PublishAck;
import java.io.IOException;
import org.jboss.logging.Logger;

public class EventPublisherImpl implements EventPublisher {

  private static final Logger logger = Logger.getLogger(EventPublisherImpl.class);
  private final JetStream js;
  private final ObjectMapper objectMapper;

  public EventPublisherImpl(JetStream js, ObjectMapper objectMapper) {
    this.js = js;
    this.objectMapper = objectMapper;
  }

  @Override
  public void publish(EventMessage<?> message) {
    String subject = "USER_EVENT." + message.userId();
    String payload;

    try {
      payload = objectMapper.writeValueAsString(message);
    } catch (JsonProcessingException e) {
      logger.errorf(
          "Failed to serialize event to JSON for userId=%s, msg=%s",
          message.userId(), e.getMessage(), e);
      return;
    }

    int maxRetries = 3;
    long backoffMillis = 1000; // start with 1s delay

    for (int attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        PublishAck ack = js.publish(subject, payload.getBytes());
        logger.infof(
            "Published event to JetStream: subject=%s, seq=%s, userId=%s",
            subject, ack.getSeqno(), message.userId());
        return;

      } catch (IOException | JetStreamApiException e) {
        logger.warnf(
            "Attempt %s/%s failed to publish to subject=%s (userId=%s): %s",
            attempt, maxRetries, subject, message.userId(), e.getMessage());

        if (attempt == maxRetries) {
          logger.errorf(
              "Giving up after %s attempts for userId=%s, subject=%s",
              maxRetries, message.userId(), subject, e);
          handlePublishFailure(message, e);
          break;
        }

        try {
          Thread.sleep(backoffMillis);
        } catch (InterruptedException ie) {
          Thread.currentThread().interrupt();
          return;
        }

        backoffMillis *= 2; // exponential backoff
      }
    }
  }

  private void handlePublishFailure(EventMessage<?> message, Exception e) {
    // TODO: write to a file
  }
}
