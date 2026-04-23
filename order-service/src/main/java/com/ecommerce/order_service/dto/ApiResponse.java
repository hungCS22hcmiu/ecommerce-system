package com.ecommerce.order_service.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AccessLevel;
import lombok.AllArgsConstructor;
import lombok.Getter;
import org.springframework.data.domain.Page;

@Getter
@AllArgsConstructor(access = AccessLevel.PRIVATE)
@JsonInclude(JsonInclude.Include.NON_NULL)
public class ApiResponse<T> {

    private final boolean success;
    private final T data;
    private final PageMeta meta;
    private final ErrorBody error;

    public static <T> ApiResponse<T> ok(T data) {
        return new ApiResponse<>(true, data, null, null);
    }

    public static <T> ApiResponse<java.util.List<T>> ok(Page<T> page) {
        PageMeta meta = new PageMeta(
                page.getNumber(),
                page.getSize(),
                page.getTotalElements(),
                page.getTotalPages()
        );
        return new ApiResponse<>(true, page.getContent(), meta, null);
    }

    public static <T> ApiResponse<T> error(String code, String message) {
        return new ApiResponse<>(false, null, null, new ErrorBody(code, message));
    }

    public record PageMeta(int page, int size, long totalElements, int totalPages) {}

    public record ErrorBody(String code, String message) {}
}
