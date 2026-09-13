# Repository Copilot Instructions

## Core Operating Rules
- **Token Efficiency First:** Avoid re-explaining architecture, repeating back code, or writing conversational preamble. Return direct code edits, unified diffs, or minimal executable steps.
- **Context Preservation:** Rely on active editor selection and explicitly referenced `#file` context. Do not search or read unreferenced files unless necessary.
- **Prefix Caching Optimization:** Keep common system rules consistent across requests so prompt prefixes remain identical for hardware/API caching.

## Coding & Architectural Standards
- Write clean, modular, and strongly-typed code adhering to established repository patterns.
- Do not add external dependencies without explicit instruction.
- Every function edit must include brief inline error-handling where applicable, but omit unnecessary commentary.

## Execution vs. Planning Protocols
- **In Plan Mode:** Focus strictly on steps, API boundaries, and file touchpoints. Do not output full implementation code. Save output specs to `.md` files when asked.
- **In Implementation/Agent Mode:** Follow the provided spec step-by-step. Implement only the exact files requested. Run unit tests to verify changes when tools are available.
- **Error Handling:** Always include basic error handling for edge cases and unexpected inputs, but avoid verbose commentary unless explicitly requested.
- **Testing:** Ensure that all changes are accompanied by appropriate unit tests to validate functionality and edge cases.
- **Documentation:** Update relevant documentation to reflect any changes in functionality, usage, or API contracts.
- **Code Reviews:** Actively participate in code reviews, providing constructive feedback and ensuring adherence to repository standards.