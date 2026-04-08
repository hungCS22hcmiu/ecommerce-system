package com.ecommerce.product_service.config;

import com.fasterxml.jackson.annotation.JsonTypeInfo;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.cache.RedisCacheConfiguration;
import org.springframework.data.redis.cache.RedisCacheManager;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.RedisSerializationContext;
import org.springframework.data.redis.serializer.StringRedisSerializer;

import java.time.Duration;
import java.util.Map;

@Configuration
@EnableCaching
public class RedisConfig {

    /**
     * Jackson ObjectMapper configured for Redis serialization:
     * - JavaTimeModule: handles OffsetDateTime, LocalDate, etc.
     * - DefaultTyping: embeds class name in JSON so deserialization works without knowing the type upfront.
     */
    @Bean
    public ObjectMapper redisObjectMapper() {
        ObjectMapper mapper = new ObjectMapper();
        mapper.registerModule(new JavaTimeModule());
        mapper.disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);
        mapper.activateDefaultTyping(
                LaissezFaireSubTypeValidator.instance,
                ObjectMapper.DefaultTyping.NON_FINAL,
                JsonTypeInfo.As.PROPERTY
        );
        return mapper;
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
    public RedisCacheManager cacheManager(RedisConnectionFactory connectionFactory,
                                          ObjectMapper redisObjectMapper) {
        RedisCacheConfiguration defaultConfig = baseCacheConfig(redisObjectMapper)
                .entryTtl(Duration.ofMinutes(10));

        Map<String, RedisCacheConfiguration> cacheConfigs = Map.of(
                "product",     baseCacheConfig(redisObjectMapper).entryTtl(Duration.ofMinutes(30)),
                "productList", baseCacheConfig(redisObjectMapper).entryTtl(Duration.ofMinutes(3))
        );

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(defaultConfig)
                .withInitialCacheConfigurations(cacheConfigs)
                .build();
    }
}
