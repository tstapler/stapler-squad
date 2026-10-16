package backend_test

import "go.opentelemetry.io/otel/attribute"

func attrKey(k string) attribute.Key { return attribute.Key(k) }
