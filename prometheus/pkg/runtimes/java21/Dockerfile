FROM eclipse-temurin:21-jdk-alpine
WORKDIR /app
COPY . /app
RUN javac -d /app/out /app/Handler.java /app/Launch.java
CMD ["java", "-cp", "/app/out", "Launch"]
