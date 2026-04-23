package com.playground.keycloak.util;

import java.text.SimpleDateFormat;
import java.util.Date;
import org.jboss.logging.Logger;
import org.keycloak.events.Event;
import org.keycloak.events.admin.AdminEvent;

public class EventLogger {

  private static final Logger logger = Logger.getLogger(EventLogger.class);

  public void logEvent(String eventName, Event event) {
    try {
      StringBuilder sb = new StringBuilder();
      sb.append("Event= ")
          .append(eventName)
          .append(", UserId=")
          .append(event.getUserId())
          .append(", ClientId=")
          .append(event.getClientId())
          .append(", IpAddress=")
          .append(event.getIpAddress())
          .append(", Timestamp=")
          .append(formatTimestamp(event.getTime()));

      if (event.getDetails() != null) {
        event.getDetails().forEach((k, v) -> sb.append(", ").append(k).append("=").append(v));
      }

      logger.info(sb.toString());
    } catch (Exception e) {
      logger.errorf(e, "Error logging event: %s", eventName);
    }
  }

  public void logAdminEvent(String eventName, AdminEvent event) {
    try {
      StringBuilder sb = new StringBuilder();
      sb.append("Event=")
          .append(eventName)
          .append(", ResourceId=")
          .append(event.getResourceId())
          .append(", OperationType=")
          .append(event.getOperationType())
          .append(", Realm=")
          .append(event.getRealmId())
          .append(", Admin=")
          .append(event.getAuthDetails().getUserId())
          .append(", Timestamp=")
          .append(formatTimestamp(event.getTime()));

      logger.info(sb.toString());
    } catch (Exception e) {
      logger.errorf(e, "Error logging admin event: %s", eventName);
    }
  }

  // Helper methods to print full event details
  public void logFullEvent(String eventName, Event event) {
    try {
      StringBuilder sb = new StringBuilder();
      sb.append("\n===== ").append(eventName).append(" =====\n");
      sb.append("Event Type: ").append(event.getType()).append("\n");
      sb.append("User ID: ").append(event.getUserId()).append("\n");
      sb.append("Realm ID: ").append(event.getRealmId()).append("\n");
      sb.append("Client ID: ").append(event.getClientId()).append("\n");
      sb.append("IP Address: ").append(event.getIpAddress()).append("\n");
      sb.append("Session ID: ").append(event.getSessionId()).append("\n");
      sb.append("Timestamp: ").append(event.getTime()).append("\n");
      sb.append("Error: ").append(event.getError()).append("\n");

      sb.append("Details:\n");
      if (event.getDetails() != null && !event.getDetails().isEmpty()) {
        event
            .getDetails()
            .forEach((k, v) -> sb.append("  ").append(k).append(" = ").append(v).append("\n"));
      } else {
        sb.append("  (empty)\n");
      }
      sb.append("=======================\n");

      logger.info(sb.toString());
    } catch (Exception e) {
      logger.errorf(e, "Error logging full event: %s", eventName);
    }
  }

  public void logFullAdminEvent(String eventName, AdminEvent event) {
    try {
      StringBuilder sb = new StringBuilder();
      sb.append("\n===== ").append(eventName).append(" =====\n");
      sb.append("Operation Type: ").append(event.getOperationType()).append("\n");
      sb.append("Resource Type: ").append(event.getResourceType()).append("\n");
      sb.append("Resource ID: ").append(event.getResourceId()).append("\n");
      sb.append("Resource Path: ").append(event.getResourcePath()).append("\n");
      sb.append("Realm ID: ").append(event.getRealmId()).append("\n");
      sb.append("Admin User ID: ").append(event.getAuthDetails().getUserId()).append("\n");
      sb.append("Admin Client ID: ").append(event.getAuthDetails().getClientId()).append("\n");
      sb.append("IP Address: ").append(event.getAuthDetails().getIpAddress()).append("\n");
      sb.append("Timestamp: ").append(event.getTime()).append("\n");
      sb.append("Error: ").append(event.getError()).append("\n");

      sb.append("Details:\n");
      if (event.getDetails() != null && !event.getDetails().isEmpty()) {
        event
            .getDetails()
            .forEach((k, v) -> sb.append("  ").append(k).append(" = ").append(v).append("\n"));
      } else {
        sb.append("  (empty)\n");
      }
      sb.append("=======================\n");

      logger.info(sb.toString());
    } catch (Exception e) {
      logger.errorf(e, "Error logging full admin event: %s", eventName);
    }
  }

  private String formatTimestamp(long timestamp) {
    return new SimpleDateFormat("yyyy-MM-dd HH:mm:ss").format(new Date(timestamp));
  }
}
