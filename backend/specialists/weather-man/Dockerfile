FROM python:3.12-slim

WORKDIR /app

COPY scripts/ ./scripts/

# SCRIPT: which script to run (get-weather or fetch-forecast)
# LOCATION: the location to query (e.g. "Boston")
ENV SCRIPT=get-weather
ENV LOCATION=Boston

ENTRYPOINT ["sh", "-c", "python scripts/${SCRIPT}.py ${LOCATION}"]
