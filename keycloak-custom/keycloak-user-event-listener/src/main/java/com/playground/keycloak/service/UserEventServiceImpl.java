package com.playground.keycloak.service;

import com.playground.keycloak.dto.EventMessage;
import com.playground.keycloak.dto.UpdatedDetails;
import com.playground.keycloak.enums.KeycloakEventType;
import com.playground.keycloak.enums.KeycloakOperation;
import com.playground.keycloak.publisher.EventPublisher;
import com.playground.keycloak.util.EventLogger;
import java.util.Map;
import org.keycloak.events.Event;
import org.keycloak.events.admin.AdminEvent;

public class UserEventServiceImpl implements UserEventService {

  private final EventLogger eventLogger;
  private final EventPublisher publisher;

  public UserEventServiceImpl(EventLogger eventLogger, EventPublisher publisher) {
    this.eventLogger = eventLogger;
    this.publisher = publisher;
  }

  public void handleUserEvent(Event event) {
    eventLogger.logEvent(event.getType().name(), event);

    var msg = mapToUserEventMsg(event);
    publisher.publish(msg);
  }

  public void handleAdminEvent(AdminEvent event) {
    eventLogger.logAdminEvent(event.getOperationType().name(), event);

    var msg = mapToUserEventMsg(event);
    publisher.publish(msg);
  }

  private EventMessage mapToUserEventMsg(Event event) {
    KeycloakOperation operation = KeycloakOperation.getByKeycloakEventType(event.getType());
    UpdatedDetails updatedDetails =
        switch (event.getType()) {
          case UPDATE_EMAIL, UPDATE_PROFILE -> mapToUpdatedDetails(event.getDetails());
          default -> null;
        };

    return new EventMessage(
        KeycloakEventType.USER_EVENT, operation, event.getUserId(), updatedDetails);
  }

  private UpdatedDetails mapToUpdatedDetails(Map<String, String> details) {
    String updatedFirstName = details.getOrDefault("updated_first_name", null);
    String updatedLastName = details.getOrDefault("updated_last_name", null);
    String updatedEmail = details.getOrDefault("updated_email", null);

    return new UpdatedDetails(updatedFirstName, updatedLastName, updatedEmail);
  }

  private EventMessage mapToUserEventMsg(AdminEvent event) {
    KeycloakOperation operation =
        KeycloakOperation.getByKeycloakOperationType(event.getOperationType());

    return new EventMessage(KeycloakEventType.ADMIN_EVENT, operation, event.getResourceId(), null);
  }
}
