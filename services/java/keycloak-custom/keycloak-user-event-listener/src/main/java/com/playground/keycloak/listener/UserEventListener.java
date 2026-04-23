package com.playground.keycloak.listener;

import com.playground.keycloak.service.UserEventService;
import org.keycloak.events.Event;
import org.keycloak.events.EventListenerProvider;
import org.keycloak.events.admin.AdminEvent;
import org.keycloak.events.admin.OperationType;
import org.keycloak.events.admin.ResourceType;

public class UserEventListener implements EventListenerProvider {

  private final UserEventService userEventService;

  public UserEventListener(UserEventService handler) {
    this.userEventService = handler;
  }

  @Override
  public void onEvent(Event event) {
    if (event == null) {
      return;
    }

    switch (event.getType()) {
      case REGISTER:
      case UPDATE_PROFILE:
      case UPDATE_EMAIL:
      case DELETE_ACCOUNT:
        userEventService.handleUserEvent(event);
        break;
      default:
        break;
    }
  }

  @Override
  public void onEvent(AdminEvent event, boolean includeRepresentation) {
    if (event == null) {
      return;
    }

    ResourceType resourceType = event.getResourceType();
    OperationType operationType = event.getOperationType();

    if (ResourceType.USER.equals(resourceType)) {
      switch (operationType) {
        case CREATE:
        case UPDATE:
        case DELETE:
          userEventService.handleAdminEvent(event);
          break;
        default:
          break;
      }
    }
  }

  @Override
  public void close() {}
}
