package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.kafka.core.KafkaTemplate;

import java.math.BigDecimal;
import java.util.List;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.verify;

@ExtendWith(MockitoExtension.class)
class OrderEventProducerTest {

    @Mock  private KafkaTemplate<String, Object> kafkaTemplate;
    @InjectMocks private OrderEventProducer producer;

    @Test
    @SuppressWarnings("unchecked")
    void publishOrderCreated_sendsToCorrectTopicWithOrderIdAsKey() {
        UUID orderId = UUID.randomUUID();
        OrderCreatedEvent event = OrderCreatedEvent.builder()
                .orderId(orderId)
                .userId(UUID.randomUUID())
                .totalAmount(BigDecimal.valueOf(150))
                .items(List.of())
                .build();

        producer.publishOrderCreated(event);

        ArgumentCaptor<ProducerRecord<String, Object>> recordCaptor =
                ArgumentCaptor.forClass(ProducerRecord.class);
        verify(kafkaTemplate).send(recordCaptor.capture());

        ProducerRecord<String, Object> record = recordCaptor.getValue();
        assertThat(record.topic()).isEqualTo("orders.created");
        assertThat(record.key()).isEqualTo(orderId.toString());
        assertThat(record.value()).isEqualTo(event);
        assertThat(record.headers().lastHeader("X-Correlation-ID")).isNotNull();
    }
}
