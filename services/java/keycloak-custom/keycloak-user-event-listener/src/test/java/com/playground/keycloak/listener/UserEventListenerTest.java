package com.playground.keycloak.listener;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import com.playground.keycloak.service.UserEventService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.keycloak.events.Event;
import org.keycloak.events.EventType;
import org.keycloak.events.admin.AdminEvent;
import org.keycloak.events.admin.OperationType;
import org.keycloak.events.admin.ResourceType;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class UserEventListenerTest {

  @Mock private UserEventService userEventService;
  @Mock private Event event;
  @Mock private AdminEvent adminEvent;
  private UserEventListener userEventListener;

  @BeforeEach
  void setUp() {
    userEventListener = new UserEventListener(userEventService);
  }

  @Test
  void onEvent_whenRegisterEvent_shouldHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.REGISTER);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService).handleUserEvent(event);
  }

  @Test
  void onEvent_whenUpdateProfileEvent_shouldHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.UPDATE_PROFILE);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService).handleUserEvent(event);
  }

  @Test
  void onEvent_whenUpdateEmailEvent_shouldHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.UPDATE_EMAIL);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService).handleUserEvent(event);
  }

  @Test
  void onEvent_whenDeleteAccountEvent_shouldHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.DELETE_ACCOUNT);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService).handleUserEvent(event);
  }

  @Test
  void onEvent_whenLoginEvent_shouldNotHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.LOGIN);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService, never()).handleUserEvent(any());
  }

  @Test
  void onEvent_whenLogoutEvent_shouldNotHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.LOGOUT);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService, never()).handleUserEvent(any());
  }

  @Test
  void onEvent_whenCodeToTokenEvent_shouldNotHandleEvent() {
    // Arrange
    when(event.getType()).thenReturn(EventType.CODE_TO_TOKEN);

    // Act
    userEventListener.onEvent(event);

    // Assert
    verify(userEventService, never()).handleUserEvent(any());
  }

  @Test
  void onEvent_whenNullEvent_shouldNotThrowException() {
    // Act & Assert
    assertDoesNotThrow(() -> userEventListener.onEvent(null));
    verify(userEventService, never()).handleUserEvent(any());
  }

  // Tests for Admin Events (onEvent with includeRepresentation)

  @Test
  void onAdminEvent_whenUserCreateEvent_shouldHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.USER);
    when(adminEvent.getOperationType()).thenReturn(OperationType.CREATE);

    // Act
    userEventListener.onEvent(adminEvent, false);

    // Assert
    verify(userEventService).handleAdminEvent(adminEvent);
  }

  @Test
  void onAdminEvent_whenUserUpdateEvent_shouldHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.USER);
    when(adminEvent.getOperationType()).thenReturn(OperationType.UPDATE);

    // Act
    userEventListener.onEvent(adminEvent, true);

    // Assert
    verify(userEventService).handleAdminEvent(adminEvent);
  }

  @Test
  void onAdminEvent_whenUserDeleteEvent_shouldHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.USER);
    when(adminEvent.getOperationType()).thenReturn(OperationType.DELETE);

    // Act
    userEventListener.onEvent(adminEvent, false);

    // Assert
    verify(userEventService).handleAdminEvent(adminEvent);
  }

  @Test
  void onAdminEvent_whenUserActionOperation_shouldNotHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.USER);
    when(adminEvent.getOperationType()).thenReturn(OperationType.ACTION);

    // Act
    userEventListener.onEvent(adminEvent, false);

    // Assert
    verify(userEventService, never()).handleAdminEvent(any());
  }

  @Test
  void onAdminEvent_whenClientCreateEvent_shouldNotHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.CLIENT);
    when(adminEvent.getOperationType()).thenReturn(OperationType.CREATE);

    // Act
    userEventListener.onEvent(adminEvent, false);

    // Assert
    verify(userEventService, never()).handleAdminEvent(any());
  }

  @Test
  void onAdminEvent_whenRealmCreateEvent_shouldNotHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.REALM);
    when(adminEvent.getOperationType()).thenReturn(OperationType.CREATE);

    // Act
    userEventListener.onEvent(adminEvent, false);

    // Assert
    verify(userEventService, never()).handleAdminEvent(any());
  }

  @Test
  void onAdminEvent_whenGroupUpdateEvent_shouldNotHandleEvent() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.GROUP);
    when(adminEvent.getOperationType()).thenReturn(OperationType.UPDATE);

    // Act
    userEventListener.onEvent(adminEvent, true);

    // Assert
    verify(userEventService, never()).handleAdminEvent(any());
  }

  @Test
  void onAdminEvent_whenNullEvent_shouldNotThrowException() {
    // Act & Assert
    assertDoesNotThrow(() -> userEventListener.onEvent(null, false));
    verify(userEventService, never()).handleAdminEvent(any());
  }

  @Test
  void onAdminEvent_whenIncludeRepresentationParameter_shouldNotAffectHandling() {
    // Arrange
    when(adminEvent.getResourceType()).thenReturn(ResourceType.USER);
    when(adminEvent.getOperationType()).thenReturn(OperationType.CREATE);

    // Act - test with both true and false
    userEventListener.onEvent(adminEvent, true);
    userEventListener.onEvent(adminEvent, false);

    // Assert - should handle both the same way
    verify(userEventService, times(2)).handleAdminEvent(adminEvent);
  }

  @Test
  void close_whenCalled_shouldNotThrowException() {
    // Act & Assert
    assertDoesNotThrow(() -> userEventListener.close());
  }

  @Test
  void constructor_whenCalled_shouldInitializeFieldsCorrectly() {
    // Act & Assert
    assertDoesNotThrow(() -> new UserEventListener(userEventService));
  }
}
