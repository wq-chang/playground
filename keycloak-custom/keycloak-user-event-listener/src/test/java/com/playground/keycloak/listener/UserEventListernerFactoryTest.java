package com.playground.keycloak.listener;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.atLeastOnce;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.mockStatic;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import io.nats.client.Connection;
import io.nats.client.JetStream;
import io.nats.client.Nats;
import java.io.IOException;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.keycloak.Config;
import org.keycloak.events.EventListenerProvider;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;
import org.mockito.Mock;
import org.mockito.MockedStatic;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class UserEventListenerFactoryTest {

  @Mock private Config.Scope config;
  @Mock private Connection natsConnection;
  @Mock private JetStream jetStream;
  @Mock private KeycloakSession keycloakSession;
  @Mock private KeycloakSessionFactory keycloakSessionFactory;

  private UserEventListenerFactory factory;
  private MockedStatic<Nats> natsMockedStatic;

  @BeforeEach
  void setUp() {
    factory = new UserEventListenerFactory();
    natsMockedStatic = mockStatic(Nats.class);
  }

  @AfterEach
  void tearDown() {
    if (natsMockedStatic != null) {
      natsMockedStatic.close();
    }
  }

  @Test
  void testGetId_shouldReturnCorrectProviderId() {
    // Act
    String id = factory.getId();

    // Assert
    assertEquals("user-event-listener", id);
  }

  @Test
  void testInit_withValidNatsUrl_shouldConnectSuccessfully() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    // Act & Assert
    assertDoesNotThrow(() -> factory.init(config));
    natsMockedStatic.verify(() -> Nats.connect(natsUrl), times(1));
  }

  @Test
  void testInit_withNullNatsUrl_shouldThrowException() {
    // Arrange
    when(config.get("nats_url")).thenReturn(null);

    // Act & Assert
    RuntimeException exception = assertThrows(RuntimeException.class, () -> factory.init(config));
    assertEquals("NATS URL not configured for SPI", exception.getMessage());
  }

  @Test
  void testInit_withEmptyNatsUrl_shouldThrowException() {
    // Arrange
    when(config.get("nats_url")).thenReturn("");

    // Act & Assert
    RuntimeException exception = assertThrows(RuntimeException.class, () -> factory.init(config));
    assertEquals("NATS URL not configured for SPI", exception.getMessage());
  }

  @Test
  void testInit_withConnectionFailure_shouldThrowException() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    natsMockedStatic
        .when(() -> Nats.connect(natsUrl))
        .thenThrow(new IOException("Connection failed"));

    // Act & Assert
    RuntimeException exception = assertThrows(RuntimeException.class, () -> factory.init(config));
    assertEquals("NATS connection failed — aborting Keycloak startup", exception.getMessage());
    assertInstanceOf(IOException.class, exception.getCause());
  }

  @Test
  void testCreate_afterSuccessfulInit_shouldReturnEventListenerProvider() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    when(natsConnection.jetStream()).thenReturn(jetStream);
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    factory.init(config);

    // Act
    EventListenerProvider provider = factory.create(keycloakSession);

    // Assert
    assertNotNull(provider);
    assertInstanceOf(UserEventListener.class, provider);
    verify(natsConnection, times(1)).jetStream();
  }

  @Test
  void testCreate_withJetStreamIOException_shouldThrowException() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    when(natsConnection.jetStream()).thenThrow(new IOException("JetStream failed"));
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    factory.init(config);

    // Act & Assert
    RuntimeException exception =
        assertThrows(RuntimeException.class, () -> factory.create(keycloakSession));
    assertEquals(
        "JetStream Initialization failed - aborting Keycloak startup", exception.getMessage());
    assertInstanceOf(IOException.class, exception.getCause());
  }

  @Test
  void testCreate_multipleInvocations_shouldCreateMultipleInstances() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    when(natsConnection.jetStream()).thenReturn(jetStream);
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    factory.init(config);

    // Act
    EventListenerProvider provider1 = factory.create(keycloakSession);
    EventListenerProvider provider2 = factory.create(keycloakSession);

    // Assert
    assertNotNull(provider1);
    assertNotNull(provider2);
    assertNotSame(provider1, provider2); // Different instances
    verify(natsConnection, times(2)).jetStream();
  }

  @Test
  void testPostInit_shouldNotThrowException() {
    // Act & Assert
    assertDoesNotThrow(() -> factory.postInit(keycloakSessionFactory));
  }

  @Test
  void testClose_withActiveConnection_shouldCloseConnection() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    factory.init(config);

    // Act
    factory.close();

    // Assert
    verify(natsConnection, times(1)).close();
  }

  @Test
  void testClose_withConnectionCloseException_shouldHandleGracefully() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    doThrow(new InterruptedException("Close interrupted")).when(natsConnection).close();
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    factory.init(config);

    // Act & Assert
    assertDoesNotThrow(() -> factory.close());
    verify(natsConnection, times(1)).close();
  }

  @Test
  void testClose_withoutInit_shouldNotThrowException() {
    // Act & Assert
    assertDoesNotThrow(() -> factory.close());
  }

  @Test
  void testClose_calledMultipleTimes_shouldHandleGracefully() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    factory.init(config);

    // Act
    factory.close();
    factory.close(); // Second call

    // Assert - close should be called at least once
    verify(natsConnection, atLeastOnce()).close();
  }

  @Test
  void testInit_withWhitespaceNatsUrl_shouldThrowException() {
    // Arrange
    when(config.get("nats_url")).thenReturn("  ");

    // Act & Assert
    RuntimeException exception = assertThrows(RuntimeException.class, () -> factory.init(config));
    assertEquals("NATS URL not configured for SPI", exception.getMessage());
  }

  @Test
  void testLifecycle_fullWorkflow() throws Exception {
    // Arrange
    String natsUrl = "nats://localhost:4222";
    when(config.get("nats_url")).thenReturn(natsUrl);
    when(natsConnection.jetStream()).thenReturn(jetStream);
    natsMockedStatic.when(() -> Nats.connect(natsUrl)).thenReturn(natsConnection);

    // Act & Assert - Full lifecycle
    assertDoesNotThrow(() -> factory.init(config));
    assertDoesNotThrow(() -> factory.postInit(keycloakSessionFactory));

    EventListenerProvider provider = factory.create(keycloakSession);
    assertNotNull(provider);

    assertDoesNotThrow(() -> factory.close());

    verify(natsConnection, times(1)).close();
  }
}
