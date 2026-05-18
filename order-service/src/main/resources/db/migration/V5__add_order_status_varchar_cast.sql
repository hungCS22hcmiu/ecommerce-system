-- Allow Hibernate to compare order_status enum columns using string literals/parameters.
-- Derived JPA queries bind enum values as character varying; without this cast
-- PostgreSQL raises "operator does not exist: order_status = character varying".
CREATE CAST (character varying AS order_status) WITH INOUT AS IMPLICIT;
