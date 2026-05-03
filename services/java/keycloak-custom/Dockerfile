ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak:26.6.1
ARG MAVEN_IMAGE=maven:3.9.15-eclipse-temurin-21-alpine

FROM ${MAVEN_IMAGE} AS jar-builder

WORKDIR /app

# Copy parent pom files (full hierarchy)
COPY pom.xml .
COPY keycloak-custom/pom.xml ./keycloak-custom/
COPY keycloak-custom/keycloak-user-event-listener/pom.xml ./keycloak-custom/keycloak-user-event-listener/

# Download dependencies
RUN mvn dependency:go-offline -B -f keycloak-custom/pom.xml

# Copy source code
COPY keycloak-custom/keycloak-user-event-listener/src ./keycloak-custom/keycloak-user-event-listener/src

# Build the SPI JAR
RUN mvn clean package -DskipTests -f keycloak-custom/pom.xml

FROM ${KEYCLOAK_IMAGE} AS builder

COPY --from=jar-builder /app/keycloak-custom/keycloak-user-event-listener/target/keycloak-user-event-listener-1.0.0.jar /opt/keycloak/providers/
RUN /opt/keycloak/bin/kc.sh build

FROM ${KEYCLOAK_IMAGE}

COPY --from=builder /opt/keycloak/providers/ /opt/keycloak/providers/

