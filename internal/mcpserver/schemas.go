package mcpserver

import "encoding/json"

var listResourcesInputSchema = json.RawMessage(`{"type":"object","additionalProperties":false}`)

var listResourcesOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "resources": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "pattern": "^[a-z][a-z0-9-]{0,62}$"},
          "kind": {"type": "string"},
          "description": {"type": "string"},
          "capabilities": {"type": "array", "items": {"type": "string"}}
        },
        "required": ["name", "kind", "description", "capabilities"],
        "additionalProperties": false
      }
    }
  },
  "required": ["resources"],
  "additionalProperties": false
}`)
