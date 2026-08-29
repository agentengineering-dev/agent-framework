# Agent Framework

This project is an agent-framework for building code agents.

## Getting Started

### Prerequisites

-   Go 1.24 or later.

### Configuration

1.  Clone the repository.
2.  Create a `.env` file in the root directory with your API keys:

    ```env
    OPENAI_API_KEY=your_openai_key
    ANTHROPIC_API_KEY=your_anthropic_key
    GOOGLE_API_KEY=your_google_key

    # unsloth studio (self hosted, OpenAI compatible)
    UNSLOTH_API_HOST=http://127.0.0.1:8888
    UNSLOTH_API_KEY=your_unsloth_key
    UNSLOTH_MODEL=unsloth/Qwen3.8-27B-GGUF
    ```

    *Note: You only need the key for the provider you intend to use.*

    For `unsloth`, `UNSLOTH_API_HOST` defaults to `http://127.0.0.1:8888` and the
    `/v1` suffix is added automatically. `UNSLOTH_API_KEY` is the key minted by
    unsloth studio, and `UNSLOTH_MODEL` selects one of the models the studio has
    loaded (`GET /v1/models` lists them).

### Usage

Run the agent using the following command:

```bash
go run main.go -goal "Your goal here" -provider "provider_name"
```

**Parameters:**

-   `-goal`: Description of what you want the agent to do.
-   `-provider`: The LLM provider to use. Options: `openai`, `anthropic`, `google`, `deepseek`, `ollama`, `unsloth`.

**Example:**

```bash
go run main.go -goal "List all files in the tool directory" -provider "openai"
```

## License

This project is licensed under the GNU Affero General Public License v3.0. See the [LICENSE.txt](LICENSE.txt) file for details.
