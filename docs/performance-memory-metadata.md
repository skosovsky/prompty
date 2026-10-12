# Parsed message metadata contract

Message metadata is constructor-owned, immutable canonical JSON. Construction
continues to decode with `JSONDocumentAsMap` (including `json.Number`) and encode
with `MapToJSONDocument` once. This preserves sorted object keys, whitespace and
string escaping normalization, duplicate-key handling and exact numeric tokens.
Absent or zero-length metadata renders as nil; an empty object renders as `{}`.
JSON null, non-object documents and incomplete/invalid JSON retain constructor
validation errors with the message index and `ErrTemplateParse` wrapping. Content
parsing still precedes metadata validation for each message.

The parsed snapshot never aliases constructor input or the public `Messages`
field. `CloneTemplate` may share the private parsed snapshot because it is
immutable; each render clones its bytes before returning them. Mutating public
metadata on either template or any rendered execution cannot affect another
render, clone or already-returned execution. No encoding is deferred to Plan or
Execute: rendering pays only the output-owned byte copy.

Permanent tests cover normalization, exact numbers, constructor errors, mutation
of constructor input and public original/clone fields, concurrent renders and
independent rendered outputs. Cold construction and warm rendering benchmarks
use the audit's constant text and 0/16/1024-key string-valued metadata fixtures.
