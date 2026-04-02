package com.ecommerce.product_service.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@NoArgsConstructor
@AllArgsConstructor
@Builder
public class ProductImageResponse {

    private Long id;
    private String url;
    private String altText;
    private int sortOrder;
}
