package com.playground.keycloak.dto;

import com.playground.keycloak.enums.KeycloakEventType;

public record EventMessage<T>(KeycloakEventType keycloakEventType, String userId, T data) {}
;
