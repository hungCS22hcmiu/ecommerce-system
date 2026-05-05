package com.ecommerce.order_service.kafka;

import com.ecommerce.order_service.kafka.event.OrderCreatedEvent;
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
    void publishOrderCreated_sendsToCorrectTopicWithOrderIdAsKey() {
        UUID orderId = UUID.randomUUID();
        OrderCreatedEvent event = OrderCreatedEvent.builder()
                .orderId(orderId)
                .userId(UUID.randomUUID())
                .totalAmount(BigDecimal.valueOf(150))
                .items(List.of())
                .build();

        producer.publishOrderCreated(event);

        @SuppressWarnings("unchecked")
        ArgumentCaptor<String> topicCaptor = ArgumentCaptor.forClass(String.class);
        @SuppressWarnings("unchecked")
        ArgumentCaptor<String> keyCaptor   = ArgumentCaptor.forClass(String.class);
        @SuppressWarnings("unchecked")
        ArgumentCaptor<Object> valueCaptor = ArgumentCaptor.forClass(Object.class);

        verify(kafkaTemplate).send(topicCaptor.capture(), keyCaptor.capture(), valueCaptor.capture());

        assertThat(topicCaptor.getValue()).isEqualTo("orders.created");
        assertThat(keyCaptor.getValue()).isEqualTo(orderId.toString());
        assertThat(valueCaptor.getValue()).isEqualTo(event);
    }
}
