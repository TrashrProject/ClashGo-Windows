FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY reqngo ./reqngo
RUN useradd -r -u 10001 guardian && mkdir -p /data && chown 10001:10001 /data
USER guardian
ENV GUARDIAN_DATA_DIR=/data
EXPOSE 8765
CMD ["uvicorn", "reqngo.server:app", "--host", "0.0.0.0", "--port", "8765", "--workers", "1"]
