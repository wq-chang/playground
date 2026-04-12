package com.playground.keycloak.dto;

import com.playground.keycloak.enums.UserEventType;
import com.playground.keycloak.enums.UserOperation;

public record EventMessage(
    UserEventType eventType,
    UserOperation operation,
    String userId,
    UpdatedDetails updatedDetails) {}
