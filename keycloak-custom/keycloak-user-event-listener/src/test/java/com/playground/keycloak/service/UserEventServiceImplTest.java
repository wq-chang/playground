package com.playground.keycloak.service;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.inOrder;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import com.playground.keycloak.dto.EventMessage;
import com.playground.keycloak.dto.UserEvent;
import com.playground.keycloak.enums.KeycloakEventType;
import com.playground.keycloak.publisher.EventPublisher;
import com.playground.keycloak.util.EventLogger;
import java.util.HashMap;
import java.util.Map;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.keycloak.events.Event;
import org.keycloak.events.EventType;
import org.keycloak.events.admin.AdminEvent;
import org.keycloak.events.admin.OperationType;
import org.mockito.ArgumentCaptor;
import org.mockito.Captor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class UserEventServiceImplTest {

  @Mock private EventLogger eventLogger;
  @Mock private EventPublisher publisher;
  @Mock private Event event;
  @Mock private AdminEvent adminEvent;
  @Captor private ArgumentCaptor<EventMessage<?>> eventMessageCaptor;
  private UserEventServiceImpl userEventService;

  @BeforeEach
  void setUp() {
    userEventService = new UserEventServiceImpl(eventLogger, publisher);
  }

  @Test
  void testHandleUserEvent_shouldLogAndPublishEvent() {
    // Arrange
    String userId = "user-123";
    EventType eventType = EventType.UPDATE_PROFILE;
    Map<String, String> details = new HashMap<>();
    details.put("username", "testuser");
    details.put("ip", "192.168.1.1");

    when(event.getUserId()).thenReturn(userId);
    when(event.getType()).thenReturn(eventType);
    when(event.getDetails()).thenReturn(details);

    // Act
    userEventService.handleUserEvent(event);

    // Assert
    verify(eventLogger).logEvent(eq("UPDATE_PROFILE"), eq(event));
    verify(publisher).publish(eventMessageCaptor.capture());

    EventMessage<?> capturedMessage = eventMessageCaptor.getValue();
    assertEquals(KeycloakEventType.USER_EVENT, capturedMessage.keycloakEventType());
    assertEquals(userId, capturedMessage.userId());
    assertNotNull(capturedMessage.data());
    assertTrue(capturedMessage.data() instanceof UserEvent);

    UserEvent userEventData = (UserEvent) capturedMessage.data();
    assertEquals(eventType, userEventData.eventType());
    assertEquals(details, userEventData.details());
  }

  @Test
  void testHandleUserEvent_withDifferentEventTypes() {
    // Arrange
    String userId = "user-456";
    EventType eventType = EventType.REGISTER;
    Map<String, String> details = new HashMap<>();
    details.put("email", "test@example.com");

    when(event.getUserId()).thenReturn(userId);
    when(event.getType()).thenReturn(eventType);
    when(event.getDetails()).thenReturn(details);

    // Act
    userEventService.handleUserEvent(event);

    // Assert
    verify(eventLogger).logEvent(eq("REGISTER"), eq(event));
    verify(publisher).publish(any(EventMessage.class));
  }

  @Test
  void testHandleUserEvent_withNullDetails() {
    // Arrange
    String userId = "user-789";
    EventType eventType = EventType.UPDATE_EMAIL;

    when(event.getUserId()).thenReturn(userId);
    when(event.getType()).thenReturn(eventType);
    when(event.getDetails()).thenReturn(null);

    // Act
    userEventService.handleUserEvent(event);

    // Assert
    verify(eventLogger).logEvent(eq("UPDATE_EMAIL"), eq(event));
    verify(publisher).publish(eventMessageCaptor.capture());

    EventMessage<?> capturedMessage = eventMessageCaptor.getValue();
    assertEquals(KeycloakEventType.USER_EVENT, capturedMessage.keycloakEventType());

    UserEvent userEventData = (UserEvent) capturedMessage.data();
    assertNull(userEventData.details());
  }

  @Test
  void testHandleAdminEvent_shouldLogAndPublishEvent() {
    // Arrange
    String resourceId = "resource-123";
    OperationType operationType = OperationType.CREATE;

    when(adminEvent.getResourceId()).thenReturn(resourceId);
    when(adminEvent.getOperationType()).thenReturn(operationType);

    // Act
    userEventService.handleAdminEvent(adminEvent);

    // Assert
    verify(eventLogger).logAdminEvent(eq("CREATE"), eq(adminEvent));
    verify(publisher).publish(eventMessageCaptor.capture());

    EventMessage<?> capturedMessage = eventMessageCaptor.getValue();
    assertEquals(KeycloakEventType.ADMIN_EVENT, capturedMessage.keycloakEventType());
    assertEquals(resourceId, capturedMessage.userId());
    assertNotNull(capturedMessage.data());
    assertTrue(capturedMessage.data() instanceof com.playground.keycloak.dto.AdminEvent);

    com.playground.keycloak.dto.AdminEvent adminEventData =
        (com.playground.keycloak.dto.AdminEvent) capturedMessage.data();
    assertEquals(operationType, adminEventData.operationType());
  }

  @Test
  void testHandleAdminEvent_withUpdateOperation() {
    // Arrange
    String resourceId = "resource-456";
    OperationType operationType = OperationType.UPDATE;

    when(adminEvent.getResourceId()).thenReturn(resourceId);
    when(adminEvent.getOperationType()).thenReturn(operationType);

    // Act
    userEventService.handleAdminEvent(adminEvent);

    // Assert
    verify(eventLogger).logAdminEvent(eq("UPDATE"), eq(adminEvent));
    verify(publisher).publish(any(EventMessage.class));
  }

  @Test
  void testHandleAdminEvent_withDeleteOperation() {
    // Arrange
    String resourceId = "resource-789";
    OperationType operationType = OperationType.DELETE;

    when(adminEvent.getResourceId()).thenReturn(resourceId);
    when(adminEvent.getOperationType()).thenReturn(operationType);

    // Act
    userEventService.handleAdminEvent(adminEvent);

    // Assert
    verify(eventLogger).logAdminEvent(eq("DELETE"), eq(adminEvent));
    verify(publisher).publish(eventMessageCaptor.capture());

    EventMessage<?> capturedMessage = eventMessageCaptor.getValue();
    assertEquals(KeycloakEventType.ADMIN_EVENT, capturedMessage.keycloakEventType());
    assertEquals(resourceId, capturedMessage.userId());

    com.playground.keycloak.dto.AdminEvent adminEventData =
        (com.playground.keycloak.dto.AdminEvent) capturedMessage.data();
    assertEquals(operationType, adminEventData.operationType());
  }

  @Test
  void testConstructor_initializesFieldsCorrectly() {
    // Act
    UserEventServiceImpl service = new UserEventServiceImpl(eventLogger, publisher);

    // Assert
    assertNotNull(service);
  }

  @Test
  void testHandleUserEvent_ensuresCorrectInteractionOrder() {
    // Arrange
    String userId = "user-order-test";
    EventType eventType = EventType.LOGIN;

    when(event.getUserId()).thenReturn(userId);
    when(event.getType()).thenReturn(eventType);
    when(event.getDetails()).thenReturn(new HashMap<>());

    // Act
    userEventService.handleUserEvent(event);

    // Assert - verify logging happens before publishing
    var inOrder = inOrder(eventLogger, publisher);
    inOrder.verify(eventLogger).logEvent(anyString(), any(Event.class));
    inOrder.verify(publisher).publish(any(EventMessage.class));
  }

  @Test
  void testHandleAdminEvent_ensuresCorrectInteractionOrder() {
    // Arrange
    String resourceId = "resource-order-test";
    OperationType operationType = OperationType.CREATE;

    when(adminEvent.getResourceId()).thenReturn(resourceId);
    when(adminEvent.getOperationType()).thenReturn(operationType);

    // Act
    userEventService.handleAdminEvent(adminEvent);

    // Assert - verify logging happens before publishing
    var inOrder = inOrder(eventLogger, publisher);
    inOrder.verify(eventLogger).logAdminEvent(anyString(), any(AdminEvent.class));
    inOrder.verify(publisher).publish(any(EventMessage.class));
  }
}
