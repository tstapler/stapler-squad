# Architecture Research: antigravity-history-translator

## Architectural Patterns
*   **Adapter / Translator Pattern:** The core requirement is translating history between different programs and sessions. Using an Adapter pattern allows the system to read from various specific input formats and write to various specific output formats through a common internal representation.
*   **Pipeline / Filter Pattern:** The process of reading history, parsing it, applying inhibition rules (Stapler squad integration), and curating it for agent context fits naturally into a processing pipeline. Each step (parser, filter, translator, emitter) can be implemented as a modular component.
*   **Plugin Architecture:** Given the open question about specific programs involved, a plugin-based architecture for parsers and emitters would allow easy extension to new history formats without modifying the core translator logic.

## Integration Points
*   **Anti-gravity CLI:** The translator needs to integrate with the anti-gravity CLI, potentially acting as a pre-processor or post-processor for session data.
*   **Stapler Squad Systems:** Integration for applying "inhibition" rules, which likely involves querying a centralized policy engine or local configuration managed by the Stapler squad to filter out sensitive or restricted commands/context from the history.
*   **Program-Specific History Stores:** The system needs file-system or API access to the history stores of the involved programs (e.g., shell history files, agent conversation logs, custom application session states).

## Data Flow and Consistency Requirements
*   **Data Flow:** 
    1.  **Ingestion:** Read history from Source A (e.g., previous session, different program).
    2.  **Standardization:** Parse raw data into a Canonical History Event model.
    3.  **Processing:** Apply curation and inhibition rules against the canonical model.
    4.  **Translation:** Convert canonical events into the format required by Target B.
    5.  **Insertion:** Safely append or insert the translated history into Target B's store.
*   **Consistency:**
    *   **Atomic Operations:** History insertion must be atomic to avoid corrupting the target program's history file during concurrent access.
    *   **Idempotency:** Re-running the translation for the same time window should not result in duplicated history entries.
    *   **Data Integrity:** The translation process must not lose critical context required by agents, even if some fields cannot be perfectly mapped.
