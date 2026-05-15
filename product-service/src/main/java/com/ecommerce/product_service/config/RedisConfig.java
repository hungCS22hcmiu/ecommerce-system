package com.ecommerce.product_service.config;

import com.ecommerce.product_service.dto.AISearchResponse;
import com.fasterxml.jackson.annotation.JsonTypeInfo;
import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.databind.DeserializationContext;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.databind.deser.std.StdDeserializer;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.databind.module.SimpleModule;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.domain.PageImpl;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.redis.cache.RedisCacheConfiguration;
import org.springframework.data.redis.cache.RedisCacheManager;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.RedisSerializationContext;
import org.springframework.data.redis.serializer.StringRedisSerializer;

import java.io.IOException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

@Configuration
@EnableCaching
public class RedisConfig {

    // Not a @Bean — keeps this mapper out of the Spring MVC context so REST responses
    // don't inherit DefaultTyping (which causes @class to appear in HTTP responses).
    private ObjectMapper buildRedisObjectMapper() {
        ObjectMapper mapper = new ObjectMapper();
        mapper.registerModule(new JavaTimeModule());
        mapper.disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);
        mapper.activateDefaultTyping(
                LaissezFaireSubTypeValidator.instance,
                ObjectMapper.DefaultTyping.NON_FINAL,
                JsonTypeInfo.As.PROPERTY
        );
        // Records are implicitly final, so NON_FINAL skips them — add @class explicitly
        // via a mixin so the Redis serializer can round-trip AISearchResponse.
        mapper.addMixIn(AISearchResponse.class, AISearchResponseTypeMixin.class);
        // Jackson 2.19 can't instantiate PageImpl (no no-arg constructor) or
        // Collections$UnmodifiableRandomAccessList (private inner class). Register
        // a custom deserializer that reads the cached JSON and reconstructs PageImpl
        // with full pagination metadata (total, page number, page size).
        SimpleModule pageModule = new SimpleModule();
        pageModule.addDeserializer(PageImpl.class, new PageImplDeserializer());
        mapper.registerModule(pageModule);
        return mapper;
    }

    @JsonTypeInfo(use = JsonTypeInfo.Id.CLASS, include = JsonTypeInfo.As.PROPERTY, property = "@class")
    abstract static class AISearchResponseTypeMixin {}

    // Reads the type-wrapped array format produced by DefaultTyping.NON_FINAL:
    //   content: ["java.util.Collections$UnmodifiableRandomAccessList", [{...}, ...]]
    // Reconstructs PageImpl(content, PageRequest, totalElements) so pagination
    // metadata (totalElements, page, size) is preserved across cache reads.
    @SuppressWarnings({"rawtypes", "unchecked"})
    private static class PageImplDeserializer extends StdDeserializer<PageImpl<?>> {
        PageImplDeserializer() { super(PageImpl.class); }

        @Override
        public PageImpl<?> deserialize(JsonParser p, DeserializationContext ctxt) throws IOException {
            ObjectMapper mapper = (ObjectMapper) p.getCodec();
            JsonNode root = mapper.readTree(p);

            List<Object> content = new ArrayList<>();
            JsonNode contentNode = root.get("content");
            if (contentNode != null) {
                // DefaultTyping wraps lists as ["TypeName", [...items...]].
                // Detect this 2-element array pattern: first element is the type string.
                JsonNode itemsArray = (contentNode.isArray()
                        && contentNode.size() == 2
                        && contentNode.get(0).isTextual())
                        ? contentNode.get(1)
                        : contentNode;
                if (itemsArray != null && itemsArray.isArray()) {
                    for (JsonNode element : itemsArray) {
                        content.add(mapper.treeToValue(element, Object.class));
                    }
                }
            }

            int page  = root.path("number").asInt(0);
            int size  = Math.max(root.path("size").asInt(20), 1);
            long total = root.path("totalElements").asLong(content.size());

            return new PageImpl<>(content, PageRequest.of(page, size), total);
        }
    }

    /**
     * Base cache configuration shared by all caches:
     * - JSON serialization for both keys and values
     * - Null values are NOT cached (prevents caching of empty results)
     * - Key prefix: "product-service::<cacheName>::"
     */
    private RedisCacheConfiguration baseCacheConfig(ObjectMapper redisObjectMapper) {
        GenericJackson2JsonRedisSerializer valueSerializer =
                new GenericJackson2JsonRedisSerializer(redisObjectMapper);

        return RedisCacheConfiguration.defaultCacheConfig()
                .serializeKeysWith(
                        RedisSerializationContext.SerializationPair.fromSerializer(new StringRedisSerializer()))
                .serializeValuesWith(
                        RedisSerializationContext.SerializationPair.fromSerializer(valueSerializer))
                .disableCachingNullValues()
                .prefixCacheNameWith("product-service::");
    }

    /**
     * CacheManager with per-cache TTL configuration:
     * - "product"     : 30 min — single product by ID; evicted explicitly on update/delete
     * - "productList" : 3 min  — paginated listings and search results; short TTL handles staleness
     *
     * Default TTL (10 min) applies to any cache not listed above.
     */
    @Bean
    public RedisCacheManager cacheManager(RedisConnectionFactory connectionFactory) {
        ObjectMapper redisObjectMapper = buildRedisObjectMapper();
        RedisCacheConfiguration defaultConfig = baseCacheConfig(redisObjectMapper)
                .entryTtl(Duration.ofMinutes(10));

        Map<String, RedisCacheConfiguration> cacheConfigs = Map.of(
                "product",     baseCacheConfig(redisObjectMapper).entryTtl(Duration.ofMinutes(30)),
                "productList", baseCacheConfig(redisObjectMapper).entryTtl(Duration.ofMinutes(3)),
                "aiSearch",    baseCacheConfig(redisObjectMapper).entryTtl(Duration.ofMinutes(15))
        );

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(defaultConfig)
                .withInitialCacheConfigurations(cacheConfigs)
                .build();
    }
}
