"""Broker-level MQTT 5 shared-subscription smoke coverage."""

from __future__ import annotations

import os
import threading
import time
import uuid

import paho.mqtt.client as mqtt


MQTT_HOST = os.getenv("HOMENAVI_MQTT_HOST", "127.0.0.1")
MQTT_PORT = int(os.getenv("HOMENAVI_MQTT_PORT", "1883"))


def subscription_succeeded(reason_code: object) -> bool:
    value = getattr(reason_code, "value", reason_code)
    return int(value) < 128


def test_shared_subscriptions_distribute_each_message_once() -> None:
    topic = f"homenavi/smoke/shared/{uuid.uuid4().hex}"
    shared_topic = f"$share/homenavi-smoke/{topic}"
    deliveries: list[list[str]] = [[], []]
    connected = [threading.Event(), threading.Event()]
    subscribed = [threading.Event(), threading.Event()]
    clients: list[mqtt.Client] = []

    for index in range(2):
        client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=f"homenavi-shared-{index}-{uuid.uuid4().hex}", protocol=mqtt.MQTTv5)

        def on_connect(current_client, _userdata, _connect_flags, reason_code, _properties=None, *, client_index=index):
            if reason_code == 0:
                connected[client_index].set()
                current_client.subscribe(shared_topic, qos=1)

        def on_subscribe(_client, _userdata, _mid, reason_codes, _properties=None, *, client_index=index):
            if all(subscription_succeeded(reason_code) for reason_code in reason_codes):
                subscribed[client_index].set()

        def on_message(_client, _userdata, message, *, client_index=index):
            deliveries[client_index].append(message.payload.decode("utf-8"))

        client.on_connect = on_connect
        client.on_subscribe = on_subscribe
        client.on_message = on_message
        client.connect(MQTT_HOST, MQTT_PORT, keepalive=20)
        client.loop_start()
        clients.append(client)

    publisher = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=f"homenavi-publisher-{uuid.uuid4().hex}", protocol=mqtt.MQTTv5)
    publisher_connected = threading.Event()
    publisher.on_connect = lambda _client, _userdata, _connect_flags, reason_code, _properties=None: publisher_connected.set() if reason_code == 0 else None
    publisher.connect(MQTT_HOST, MQTT_PORT, keepalive=20)
    publisher.loop_start()

    try:
        assert all(event.wait(10) for event in connected), "shared consumers did not connect"
        assert all(event.wait(10) for event in subscribed), "shared consumers did not subscribe"
        assert publisher_connected.wait(10), "publisher did not connect"

        expected = [str(index) for index in range(12)]
        for payload in expected:
            result = publisher.publish(topic, payload, qos=1)
            result.wait_for_publish(timeout=10)
            assert result.is_published(), f"did not publish {payload}"

        deadline = time.monotonic() + 10
        while time.monotonic() < deadline and sum(len(items) for items in deliveries) < len(expected):
            time.sleep(0.05)

        observed = [payload for consumer_deliveries in deliveries for payload in consumer_deliveries]
        assert sorted(observed, key=int) == expected, f"shared delivery was duplicated or incomplete: {observed!r}"
        assert all(consumer_deliveries for consumer_deliveries in deliveries), f"shared delivery was not distributed: {deliveries!r}"
    finally:
        publisher.disconnect()
        publisher.loop_stop()
        for client in clients:
            client.disconnect()
            client.loop_stop()