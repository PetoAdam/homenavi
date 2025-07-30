FROM python:3.11-slim

WORKDIR /app

# Copy requirements first for better caching
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the application
COPY main.py .

# Make the script executable
RUN chmod +x main.py

EXPOSE 9000

CMD ["python", "main.py"]
