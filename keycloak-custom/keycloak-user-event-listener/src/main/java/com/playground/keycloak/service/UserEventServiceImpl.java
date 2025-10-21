package com.playground.keycloak.service;

import com.playground.keycloak.dto.EventMessage;
import com.playground.keycloak.dto.UserEvent;
import com.playground.keycloak.enums.KeycloakEventType;
import com.playground.keycloak.publisher.EventPublisher;
import com.playground.keycloak.util.EventLogger;
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

  private EventMessage<UserEvent> mapToUserEventMsg(Event event) {
    UserEvent userEvent = new UserEvent(event.getType(), event.getDetails());
    return new EventMessage<>(KeycloakEventType.USER_EVENT, event.getUserId(), userEvent);
  }

  private EventMessage<com.playground.keycloak.dto.AdminEvent> mapToUserEventMsg(AdminEvent event) {
    var adminEvent = new com.playground.keycloak.dto.AdminEvent(event.getOperationType());
    return new EventMessage<>(KeycloakEventType.ADMIN_EVENT, event.getResourceId(), adminEvent);
  }
}
