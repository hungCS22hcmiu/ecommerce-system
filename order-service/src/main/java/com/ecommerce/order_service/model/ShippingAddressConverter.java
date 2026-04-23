package com.ecommerce.order_service.model;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.persistence.AttributeConverter;
import jakarta.persistence.Converter;

@Converter
public class ShippingAddressConverter implements AttributeConverter<ShippingAddress, String> {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Override
    public String convertToDatabaseColumn(ShippingAddress address) {
        if (address == null) return null;
        try {
            return MAPPER.writeValueAsString(address);
        } catch (Exception e) {
            throw new IllegalArgumentException("Failed to serialize ShippingAddress", e);
        }
    }

    @Override
    public ShippingAddress convertToEntityAttribute(String json) {
        if (json == null || json.isBlank()) return null;
        try {
            return MAPPER.readValue(json, ShippingAddress.class);
        } catch (Exception e) {
            throw new IllegalArgumentException("Failed to deserialize ShippingAddress", e);
        }
    }
}
