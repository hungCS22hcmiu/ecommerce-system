package com.ecommerce.order_service.dto;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotEmpty;
import jakarta.validation.constraints.NotNull;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;
import java.util.UUID;

@Data
@NoArgsConstructor
@AllArgsConstructor
public class CreateOrderRequest {

    @NotNull
    private UUID cartId;

    @NotEmpty
    @Valid
    private List<OrderItemRequest> items;

    @NotNull
    @Valid
    private ShippingAddressDto shippingAddress;
}
