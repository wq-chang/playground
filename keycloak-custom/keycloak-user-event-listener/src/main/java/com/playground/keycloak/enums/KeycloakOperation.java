package com.playground.keycloak.enums;

import org.keycloak.events.EventType;
import org.keycloak.events.admin.OperationType;

public enum KeycloakOperation {
  CREATE,
  UPDATE,
  DELETE;

  public static KeycloakOperation getByKeycloakEventType(EventType eventType) {
    return switch (eventType) {
      case REGISTER -> CREATE;
      case UPDATE_EMAIL, UPDATE_PROFILE -> UPDATE;
      case DELETE_ACCOUNT -> DELETE;
      default -> null;
    };
  }

  public static KeycloakOperation getByKeycloakOperationType(OperationType operationType) {
    return switch (operationType) {
      case CREATE -> CREATE;
      case UPDATE -> UPDATE;
      case DELETE -> DELETE;
      default -> null;
    };
  }
}
