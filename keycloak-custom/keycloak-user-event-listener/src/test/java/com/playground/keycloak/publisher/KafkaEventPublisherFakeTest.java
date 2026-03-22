package com.playground.keycloak.publisher;

import static org.assertj.core.api.Assertions.assertThat;

import com.playground.keycloak.dto.EventMessage;
import com.playground.keycloak.enums.KeycloakEventType;
import com.playground.keycloak.enums.KeycloakOperation;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

public class KafkaEventPublisherFakeTest extends EventPublisherContractTest {

  private KafkaEventPublisherFake fake;

  @Override
  protected EventPublisher createPublisher() {
    return new KafkaEventPublisherFake();
  }

  @BeforeEach
  void setup() {
    fake = (KafkaEventPublisherFake) createPublisher();
  }

  @Test
  void publish_whenMessageIsValid_shouldStoreMessage() {
    EventMessage message =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            null);

    fake.publish(message);

    assertThat(fake.getPublishedMessages()).hasSize(1).containsExactly(message);
  }

  @Test
  void publish_whenMessageIsNull_shouldNotStoreMessage() {
    fake.publish(null);
    assertThat(fake.getPublishedMessages()).isEmpty();
  }

  @Test
  void publish_whenPublisherClosed_shouldNotStoreMessage() {
    fake.close();

    EventMessage message =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            null);

    fake.publish(message);
    assertThat(fake.getPublishedMessages()).isEmpty();
  }

  @Test
  void clear_whenCalled_shouldClearMessages() {
    KafkaEventPublisherFake fake = (KafkaEventPublisherFake) createPublisher();
    EventMessage message =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            null);

    fake.publish(message);
    assertThat(fake.getPublishedMessages()).hasSize(1);

    fake.clear();
    assertThat(fake.getPublishedMessages()).isEmpty();
  }

  @Test
  void getLastMessage_whenMessagesPublished_shouldReturnLastMessage() {
    EventMessage message1 =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            null);
    EventMessage message2 =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.UPDATE,
            UUID.randomUUID().toString(),
            null);

    fake.publish(message1);
    fake.publish(message2);

    assertThat(fake.getLastMessage()).isEqualTo(message2);
  }

  @Test
  void getLastMessage_whenNoMessages_shouldReturnNull() {
    assertThat(fake.getLastMessage()).isNull();
  }

  @Test
  void getMessage_whenIndexValid_shouldReturnMessage() {
    EventMessage message =
        new EventMessage(
            KeycloakEventType.USER_EVENT,
            KeycloakOperation.CREATE,
            UUID.randomUUID().toString(),
            null);

    fake.publish(message);

    assertThat(fake.getMessage(0)).isEqualTo(message);
  }

  @Test
  void getMessage_whenIndexInvalid_shouldThrowException() {
    org.assertj.core.api.Assertions.assertThatThrownBy(() -> fake.getMessage(0))
        .isInstanceOf(IndexOutOfBoundsException.class);
  }
}
