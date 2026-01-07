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
    ```

    *Note: You only need the key for the provider you intend to use.*

### Usage

Run the agent using the following command:

```bash
go run main.go -goal "Your goal here" -provider "provider_name"
```

**Parameters:**

-   `-goal`: Description of what you want the agent to do.
-   `-provider`: The LLM provider to use. Options: `openai`, `anthropic`, `google`.

**Example:**

```bash
go run main.go -goal "List all files in the tool directory" -provider "openai"
```

## License

This project is licensed under the GNU Affero General Public License v3.0. See the [LICENSE.txt](LICENSE.txt) file for details.
