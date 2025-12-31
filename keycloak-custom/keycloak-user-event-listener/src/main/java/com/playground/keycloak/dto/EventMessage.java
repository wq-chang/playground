package com.playground.keycloak.dto;

import com.playground.keycloak.enums.KeycloakEventType;
import com.playground.keycloak.enums.KeycloakOperation;

public record EventMessage(
    KeycloakEventType eventType,
    KeycloakOperation operationType,
    String userId,
    UpdatedDetails updatedDetails) {}
