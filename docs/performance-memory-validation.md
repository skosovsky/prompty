# Read-only cache validation contract

`ValidateCachePolicies` accepts precisely the public value and non-nil pointer
variants accepted by wire conversion: text, media, reasoning, tool call, and
tool result. Nil interfaces, typed nil pointers, and custom content variants
remain errors, even when no message or part has a cache policy.

Validation does not mutate input or copy content payloads. Media bytes, policy
controls, pointer representations, and nested content retain their ownership.
Controls still pass through the existing JSON object validation.

The observable error order is preserved per message: validate the message policy;
validate the complete content tree's supported representations in depth-first
order; then validate part policies in depth-first preorder. This matches the
previous forward-conversion, policy-rewrite, reverse-conversion sequence. Thus
an unsupported later part takes precedence over an earlier invalid part policy,
while an invalid message policy takes precedence over malformed content.
Errors retain the wire conversion's nested `content[i]` wrapping and cache
policy JSON pointer paths. The successful forward conversion always creates
canonical, supported wire types, so its reverse conversion introduces no further
validation failures; the read-only traversal preserves that accepted domain.

The rewrite operation retains its existing mutation and canonicalization contract.
