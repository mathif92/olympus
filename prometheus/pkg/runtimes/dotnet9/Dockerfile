FROM mcr.microsoft.com/dotnet/sdk:9.0
WORKDIR /app
COPY . /app
ENTRYPOINT ["dotnet", "run", "--project", "/app/Prometheus.csproj", "--configuration", "Release", "--no-launch-profile"]
