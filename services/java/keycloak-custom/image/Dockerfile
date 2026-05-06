ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak:26.6.1
ARG MAVEN_IMAGE=maven:3.9.15-eclipse-temurin-21-alpine

FROM ${MAVEN_IMAGE} AS jar-builder

WORKDIR /app

# Copy the Maven parent hierarchy so all SPI modules resolve from the same graph.
COPY services/java/pom.xml ./pom.xml
COPY services/java/keycloak-custom/pom.xml ./keycloak-custom/pom.xml
COPY services/java/keycloak-custom/keycloak-user-event-listener/pom.xml ./keycloak-custom/keycloak-user-event-listener/pom.xml

# Download dependencies before copying sources to maximize Docker layer reuse.
RUN mvn -B -f keycloak-custom/pom.xml dependency:go-offline --no-transfer-progress

COPY services/java/keycloak-custom/keycloak-user-event-listener/src ./keycloak-custom/keycloak-user-event-listener/src

RUN mvn -B -f keycloak-custom/pom.xml clean package -DskipTests

FROM ${KEYCLOAK_IMAGE} AS builder

COPY --from=jar-builder /app/keycloak-custom/keycloak-user-event-listener/target/keycloak-user-event-listener-1.0.0.jar /opt/keycloak/providers/
RUN /opt/keycloak/bin/kc.sh build

FROM ${KEYCLOAK_IMAGE}

COPY --from=builder /opt/keycloak/ /opt/keycloak/
