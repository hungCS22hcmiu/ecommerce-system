#!/usr/bin/env python3
"""Generate N orders.created JSON payloads, one per line, for kafka-console-producer.

Schema matches payment-service/internal/kafka/event/events.go OrderCreatedEvent.
"""
import json
import sys
import uuid


def main() -> int:
    n = int(sys.argv[1]) if len(sys.argv) > 1 else 10000
    user_id = str(uuid.uuid4())
    for _ in range(n):
        order_id = str(uuid.uuid4())
        evt = {
            "orderId": order_id,
            "userId": user_id,
            "totalAmount": "9.99",
            "items": [{"productId": 1, "quantity": 1, "price": "9.99"}],
        }
        sys.stdout.write(json.dumps(evt) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
