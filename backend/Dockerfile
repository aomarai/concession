FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY backend/bin/concession /app/concession
COPY backend/.env.example /app/.env.example

ENV PORT=8080
EXPOSE 8080

CMD ["/app/concession"]
