package com.playground.keycloak.listener;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.mockConstruction;
import static org.mockito.Mockito.when;

import org.apache.kafka.clients.producer.KafkaProducer;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.keycloak.Config;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;
import org.mockito.Mock;
import org.mockito.MockedConstruction;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class UserEventListenerFactoryTest {

  @Mock private Config.Scope config;
  @Mock private KeycloakSession keycloakSession;
  @Mock private KeycloakSessionFactory keycloakSessionFactory;

  private UserEventListenerFactory factory;

  @BeforeEach
  void setUp() {
    factory = new UserEventListenerFactory();
  }

  @Test
  void getId_whenCalled_shouldReturnCorrectProviderId() {
    assertEquals("user-event-listener", factory.getId());
  }

  @Test
  void init_whenConfigValid_shouldInitializeSuccessfully() {
    // Arrange
    setupValidConfig();

    try (MockedConstruction<KafkaProducer> mockedKafkaProducer =
        mockConstruction(KafkaProducer.class)) {
      // Act & Assert
      assertDoesNotThrow(() -> factory.init(config));
    }
  }

  @Test
  void init_whenKafkaBrokersMissing_shouldThrowException() {
    // Arrange
    when(config.get("kafka_brokers")).thenReturn(null);

    // Act & Assert
    RuntimeException exception = assertThrows(RuntimeException.class, () -> factory.init(config));
    assertEquals("kafka_brokers not configured for SPI", exception.getMessage());
  }

  @Test
  void init_whenTopicMissing_shouldThrowException() {
    // Arrange
    when(config.get("kafka_brokers")).thenReturn("localhost:9092");
    when(config.get("kafka_security_providers_config")).thenReturn("");
    when(config.get("topic")).thenReturn(null);

    // Act & Assert
    RuntimeException exception = assertThrows(RuntimeException.class, () -> factory.init(config));
    assertEquals("topic not configured for SPI", exception.getMessage());
  }

  @Test
  void create_whenInitialized_shouldReturnInstance() {
    // Arrange
    setupValidConfig();

    try (MockedConstruction<KafkaProducer> mockedKafkaProducer =
        mockConstruction(KafkaProducer.class)) {
      factory.init(config);

      // Act
      var provider = factory.create(keycloakSession);

      // Assert
      assertNotNull(provider);
    }
  }

  @Test
  void postInit_whenCalled_shouldNotThrow() {
    assertDoesNotThrow(() -> factory.postInit(keycloakSessionFactory));
  }

  @Test
  void close_whenCalled_shouldNotThrow() {
    // Arrange
    setupValidConfig();

    try (MockedConstruction<KafkaProducer> mockedKafkaProducer =
        mockConstruction(KafkaProducer.class)) {
      factory.init(config);

      // Act & Assert
      assertDoesNotThrow(() -> factory.close());
    }
  }

  private void setupValidConfig() {
    when(config.get("kafka_brokers")).thenReturn("localhost:9092");
    when(config.get("kafka_security_providers_config")).thenReturn("");
    when(config.get("topic")).thenReturn("user-events");
    when(config.getLong("close_timeout")).thenReturn(1000L);
    when(config.getInt("max_retries")).thenReturn(3);
    when(config.getLong("initial_backoff_millis")).thenReturn(100L);
    when(config.getInt("kafka_delivery_timeout_ms_config")).thenReturn(3000);
    when(config.getInt("kafka_request_timeout_ms_config")).thenReturn(3000);
    when(config.getLong("kafka_linger_ms")).thenReturn(0L);
    when(config.get("kafka_sasl_mechanism")).thenReturn("SCRAM-SHA-256");
    when(config.get("kafka_user")).thenReturn("user");
    when(config.get("kafka_password")).thenReturn("password");
  }
}
