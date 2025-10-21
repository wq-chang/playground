package com.playground.keycloak.dto;

import org.keycloak.events.admin.OperationType;

public record AdminEvent(OperationType operationType) {}
