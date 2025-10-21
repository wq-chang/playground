package com.playground.keycloak.dto;

import java.util.Map;
import org.keycloak.events.EventType;

public record UserEvent(EventType eventType, Map<String, String> details) {}
