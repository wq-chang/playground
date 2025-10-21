FROM maven:3.9.11-eclipse-temurin-21-alpine AS jar-builder

WORKDIR /app/keycloak-user-event-listener

# Build user event listener spi
COPY ./keycloak-user-event-listener/pom.xml .
RUN mvn dependency:go-offline -B

COPY ./keycloak-user-event-listener/src ./src
RUN mvn clean package -DskipTests

FROM quay.io/keycloak/keycloak:26.4.1 AS builder

COPY --from=jar-builder /app/keycloak-user-event-listener/target/keycloak-user-event-listener-1.0.0.jar /opt/keycloak/providers/
RUN /opt/keycloak/bin/kc.sh build

FROM quay.io/keycloak/keycloak:26.4.1

COPY --from=builder /opt/keycloak/providers/ /opt/keycloak/providers/
