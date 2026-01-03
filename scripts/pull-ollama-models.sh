#!/bin/bash
# =============================================================================
# pull-ollama-models.sh
# =============================================================================
# Pull required Ollama models after the container is running.
# 
# Usage:
#   ./scripts/pull-ollama-models.sh                    # Pull default model
#   ./scripts/pull-ollama-models.sh llama3.1:8b        # Pull specific model
#   ./scripts/pull-ollama-models.sh llama3.1:8b phi3:mini  # Pull multiple models
#
# For GTX 1060 6GB (production), recommended models:
#   - llama3.2:3b      (fast, good for simple tasks)
#   - phi3:mini        (3.8B, good reasoning)
#   - gemma2:2b        (very fast, basic)
#   - mistral:7b-q4    (quantized, fits in 6GB)
#
# For RTX 4070 SUPER (development), recommended models:
#   - llama3.1:8b      (balanced, good all-around)
#   - mistral:7b       (fast, capable)
#   - deepseek-coder:6.7b (coding tasks)
#   - llama3.1:70b-q4  (high quality, slower)
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default model if none specified
DEFAULT_MODEL="${OLLAMA_MODEL:-llama3.1:8b}"

# Get models from arguments or use default
if [ $# -eq 0 ]; then
    MODELS="$DEFAULT_MODEL"
else
    MODELS="$@"
fi

echo -e "${YELLOW}Ollama Model Puller${NC}"
echo "================================"

# Check if ollama container is running
if ! docker compose ps ollama | grep -q "running"; then
    echo -e "${RED}Error: Ollama container is not running.${NC}"
    echo "Start it with: docker compose up -d ollama"
    exit 1
fi

# Wait for Ollama to be ready
echo "Waiting for Ollama to be ready..."
for i in {1..30}; do
    if docker compose exec ollama ollama list &>/dev/null; then
        echo -e "${GREEN}Ollama is ready!${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}Error: Ollama did not become ready in time.${NC}"
        exit 1
    fi
    sleep 2
done

# Pull each model
for model in $MODELS; do
    echo ""
    echo -e "${YELLOW}Pulling model: ${model}${NC}"
    echo "This may take a while depending on model size and connection speed..."
    
    if docker compose exec ollama ollama pull "$model"; then
        echo -e "${GREEN}✓ Successfully pulled: ${model}${NC}"
    else
        echo -e "${RED}✗ Failed to pull: ${model}${NC}"
    fi
done

echo ""
echo "================================"
echo -e "${GREEN}Done!${NC}"
echo ""
echo "Available models:"
docker compose exec ollama ollama list
