package com.playground.keycloak.service;

import org.keycloak.events.Event;
import org.keycloak.events.admin.AdminEvent;

public interface UserEventService {

  public void handleUserEvent(Event event);

  public void handleAdminEvent(AdminEvent event);
}
