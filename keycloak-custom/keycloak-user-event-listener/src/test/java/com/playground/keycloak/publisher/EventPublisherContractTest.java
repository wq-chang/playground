package com.playground.keycloak.publisher;

import static org.assertj.core.api.Assertions.assertThatCode;

import com.playground.keycloak.dto.EventMessage;
import com.playground.keycloak.dto.UpdatedDetails;
import com.playground.keycloak.enums.KeycloakEventType;
import com.playground.keycloak.enums.KeycloakOperation;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInstance;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
public abstract class EventPublisherContractTest {

  protected abstract EventPublisher createPublisher();

  @Test
  void publish_whenMessageIsValid_shouldNotThrow() {
    EventPublisher publisher = createPublisher();
    EventMessage message =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            new UpdatedDetails("John", "Doe", "jdoe", "jdoe@example.com"));

    assertThatCode(() -> publisher.publish(message)).doesNotThrowAnyException();
  }

  @Test
  void publish_whenMessageIsNull_shouldNotThrow() {
    EventPublisher publisher = createPublisher();
    assertThatCode(() -> publisher.publish(null)).doesNotThrowAnyException();
  }

  @Test
  void close_whenCalled_shouldNotThrow() {
    EventPublisher publisher = createPublisher();
    assertThatCode(publisher::close).doesNotThrowAnyException();
  }

  @Test
  void publish_whenPublisherClosed_shouldNotThrow() throws Exception {
    EventPublisher publisher = createPublisher();
    publisher.close();

    EventMessage message =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            null);

    // Should effectively be a no-op or swallowed error
    assertThatCode(() -> publisher.publish(message)).doesNotThrowAnyException();
  }
}
