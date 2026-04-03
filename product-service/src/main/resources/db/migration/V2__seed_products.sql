-- ============================================================
-- V2: Seed data — categories + 200 sample products + images
--
-- Seller UUIDs are fixed placeholders.
-- Replace seller_a with the actual seller UUID from sample_users.sql
-- if you need ownership checks to work with that account.
--   seller_a: a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11
--   seller_b: b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11
--   seller_c: c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11
-- ============================================================

-- ── Categories (5 root + 14 leaf = IDs 1–19) ─────────────────────────────────

INSERT INTO categories (name, slug, parent_id, sort_order) VALUES
    ('Electronics',       'electronics',        NULL, 1),
    ('Clothing',          'clothing',           NULL, 2),
    ('Home & Garden',     'home-garden',        NULL, 3),
    ('Books',             'books',              NULL, 4),
    ('Sports & Outdoors', 'sports-outdoors',    NULL, 5);

INSERT INTO categories (name, slug, parent_id, sort_order) VALUES
    ('Laptops',           'laptops',            1, 1),   -- id 6
    ('Smartphones',       'smartphones',        1, 2),   -- id 7
    ('Keyboards & Mice',  'keyboards-mice',     1, 3),   -- id 8
    ('Monitors',          'monitors',           1, 4),   -- id 9
    ('Audio',             'audio',              1, 5),   -- id 10
    ('Men''s Clothing',   'mens-clothing',      2, 1),   -- id 11
    ('Women''s Clothing', 'womens-clothing',    2, 2),   -- id 12
    ('Footwear',          'footwear',           2, 3),   -- id 13
    ('Kitchen',           'kitchen',            3, 1),   -- id 14
    ('Furniture',         'furniture',          3, 2),   -- id 15
    ('Programming',       'programming-books',  4, 1),   -- id 16
    ('Fiction',           'fiction-books',      4, 2),   -- id 17
    ('Fitness',           'fitness',            5, 1),   -- id 18
    ('Outdoor Gear',      'outdoor-gear',       5, 2);   -- id 19

-- ── Products (200 rows via generate_series) ───────────────────────────────────
-- Distribution:
--   1–20   Laptops        21–40  Smartphones    41–55  Keyboards & Mice
--   56–70  Monitors       71–85  Audio          86–100 Men's Clothing
--   101–115 Women's       116–125 Footwear      126–140 Kitchen
--   141–150 Furniture     151–165 Programming   166–175 Fiction
--   176–188 Fitness       189–200 Outdoor Gear
-- Status: ~85% ACTIVE, ~10% INACTIVE (i%10=0), ~5% DELETED (i%20=0)
-- Sellers: round-robin across 3 UUIDs

INSERT INTO products (name, description, price, category_id, seller_id, status, stock_quantity, stock_reserved, version)
SELECT
    -- name
    CASE
        WHEN i BETWEEN   1 AND  20 THEN 'Laptop '           || (ARRAY['ProBook','UltraSlim','WorkStation','GamingPro','AirMax'])[(i-1)%5+1]  || ' ' || LPAD(i::text,       2,'0')
        WHEN i BETWEEN  21 AND  40 THEN 'Smartphone '       || (ARRAY['Nova','Pulse','Edge','Orbit','Flux'])[(i-21)%5+1]                     || ' ' || LPAD((i-20)::text,  2,'0')
        WHEN i BETWEEN  41 AND  55 THEN 'Keyboard '         || (ARRAY['TKL','Full','Mini60','Wireless','Numpad'])[(i-41)%5+1]                || ' ' || LPAD((i-40)::text,  2,'0')
        WHEN i BETWEEN  56 AND  70 THEN 'Monitor '          || (ARRAY['CurveX','FlatView','UltraWide','ProDisplay','GameSync'])[(i-56)%5+1]  || ' ' || LPAD((i-55)::text,  2,'0')
        WHEN i BETWEEN  71 AND  85 THEN 'Headphones '       || (ARRAY['BassMax','ClearTone','NoiseX','StudioPro','SportFit'])[(i-71)%5+1]    || ' ' || LPAD((i-70)::text,  2,'0')
        WHEN i BETWEEN  86 AND 100 THEN 'Men''s '           || (ARRAY['Oxford Shirt','Chino Pants','Polo Shirt','Slim Jeans','Blazer'])[(i-86)%5+1] || ' ' || LPAD((i-85)::text,2,'0')
        WHEN i BETWEEN 101 AND 115 THEN 'Women''s '         || (ARRAY['Floral Dress','Blouse','Cardigan','Skinny Jeans','Skirt'])[(i-101)%5+1] || ' ' || LPAD((i-100)::text,2,'0')
        WHEN i BETWEEN 116 AND 125 THEN 'Shoes '            || (ARRAY['Runner','Trainer','Casual','Hiking','Court'])[(i-116)%5+1]            || ' ' || LPAD((i-115)::text,  2,'0')
        WHEN i BETWEEN 126 AND 140 THEN 'Kitchen '          || (ARRAY['Blender','Air Fryer','Rice Cooker','Knife Set','Cutting Board'])[(i-126)%5+1] || ' ' || LPAD((i-125)::text,2,'0')
        WHEN i BETWEEN 141 AND 150 THEN 'Furniture '        || (ARRAY['Office Chair','Standing Desk','Bookshelf','Sofa','Bed Frame'])[(i-141)%5+1] || ' ' || LPAD((i-140)::text,2,'0')
        WHEN i BETWEEN 151 AND 165 THEN 'Book: '            || (ARRAY['Clean Code','System Design','DDD','Refactoring','The Pragmatic Programmer'])[(i-151)%5+1] || ' Ed.' || ((i-150-1)/5+1)
        WHEN i BETWEEN 166 AND 175 THEN 'Novel: '           || (ARRAY['The Midnight','Lost Horizon','Echo Chamber','Silver Thread','Dark Signal'])[(i-166)%5+1] || ' Vol.' || ((i-165-1)/5+1)
        WHEN i BETWEEN 176 AND 188 THEN 'Fitness '          || (ARRAY['Dumbbell Set','Yoga Mat','Resistance Bands','Pull-up Bar','Jump Rope'])[(i-176)%5+1] || ' ' || LPAD((i-175)::text,2,'0')
        WHEN i BETWEEN 189 AND 200 THEN 'Camping '          || (ARRAY['Tent','Sleeping Bag','Trekking Poles','Headlamp','Water Filter'])[(i-189)%5+1] || ' ' || LPAD((i-188)::text,2,'0')
    END,
    -- description
    CASE
        WHEN i BETWEEN   1 AND  20 THEN 'Intel Core i' || (5+(i%3)*2) || ', ' || (8+(i%4)*8) || 'GB RAM, ' || (256+(i%4)*256) || 'GB SSD, 15.6" FHD display'
        WHEN i BETWEEN  21 AND  40 THEN '5G, ' || (64+(i%4)*64) || 'GB storage, ' || (6+(i%3)) || '.5" AMOLED, ' || (4000+(i%4)*500) || 'mAh battery'
        WHEN i BETWEEN  41 AND  55 THEN (ARRAY['Cherry MX Red','Cherry MX Blue','Cherry MX Brown','Gateron Yellow','Kailh Box White'])[(i-40)%5+1] || ' switches, RGB backlit, USB-C'
        WHEN i BETWEEN  56 AND  70 THEN (24+(i%3)*4) || '" ' || (ARRAY['1080p 144Hz','1440p 165Hz','4K 60Hz'])[(i%3)+1] || ' IPS panel, 1ms response time'
        WHEN i BETWEEN  71 AND  85 THEN 'Active noise cancellation, ' || (20+(i%4)*10) || 'hr playtime, Bluetooth 5.3, foldable design'
        WHEN i BETWEEN  86 AND 100 THEN '100% cotton, regular fit, machine washable, sizes S–XXL'
        WHEN i BETWEEN 101 AND 115 THEN 'Polyester blend, relaxed fit, suitable for casual and formal wear'
        WHEN i BETWEEN 116 AND 125 THEN 'Breathable mesh upper, cushioned midsole, EU sizes 36–46'
        WHEN i BETWEEN 126 AND 140 THEN 'Stainless steel / BPA-free, dishwasher safe, ' || (1+(i%3)) || '-year warranty'
        WHEN i BETWEEN 141 AND 150 THEN 'Adjustable height, ergonomic design, weight capacity ' || (100+(i%5)*20) || 'kg'
        WHEN i BETWEEN 151 AND 165 THEN 'Paperback, ' || (200+(i-150)*15) || ' pages, updated ' || (2019+(i%5)) || ' edition'
        WHEN i BETWEEN 166 AND 175 THEN (180+(i-165)*22) || ' pages, bestseller, translated into 12 languages'
        WHEN i BETWEEN 176 AND 188 THEN 'Professional grade, suitable for home and commercial gym use'
        WHEN i BETWEEN 189 AND 200 THEN 'Waterproof, UV-resistant, lightweight, includes carry bag'
    END,
    -- price (category-specific ranges)
    ROUND(CAST(
        CASE
            WHEN i BETWEEN   1 AND  20 THEN 499  + (i * 75)  % 1501
            WHEN i BETWEEN  21 AND  40 THEN 299  + (i * 45)  %  901
            WHEN i BETWEEN  41 AND  55 THEN 29   + (i * 11)  %  171
            WHEN i BETWEEN  56 AND  70 THEN 149  + (i * 37)  %  551
            WHEN i BETWEEN  71 AND  85 THEN 29   + (i * 29)  %  471
            WHEN i BETWEEN  86 AND 100 THEN 19   + (i * 7)   %  131
            WHEN i BETWEEN 101 AND 115 THEN 25   + (i * 9)   %  176
            WHEN i BETWEEN 116 AND 125 THEN 39   + (i * 17)  %  211
            WHEN i BETWEEN 126 AND 140 THEN 9    + (i * 13)  %  191
            WHEN i BETWEEN 141 AND 150 THEN 99   + (i * 59)  %  901
            WHEN i BETWEEN 151 AND 165 THEN 9    + (i * 3)   %   51
            WHEN i BETWEEN 166 AND 175 THEN 9    + (i * 2)   %   31
            WHEN i BETWEEN 176 AND 188 THEN 19   + (i * 21)  %  281
            WHEN i BETWEEN 189 AND 200 THEN 29   + (i * 33)  %  471
        END AS numeric
    ), 2),
    -- category_id
    CASE
        WHEN i BETWEEN   1 AND  20 THEN  6
        WHEN i BETWEEN  21 AND  40 THEN  7
        WHEN i BETWEEN  41 AND  55 THEN  8
        WHEN i BETWEEN  56 AND  70 THEN  9
        WHEN i BETWEEN  71 AND  85 THEN 10
        WHEN i BETWEEN  86 AND 100 THEN 11
        WHEN i BETWEEN 101 AND 115 THEN 12
        WHEN i BETWEEN 116 AND 125 THEN 13
        WHEN i BETWEEN 126 AND 140 THEN 14
        WHEN i BETWEEN 141 AND 150 THEN 15
        WHEN i BETWEEN 151 AND 165 THEN 16
        WHEN i BETWEEN 166 AND 175 THEN 17
        WHEN i BETWEEN 176 AND 188 THEN 18
        WHEN i BETWEEN 189 AND 200 THEN 19
    END,
    -- seller_id (round-robin across 3 sellers)
    CASE i % 3
        WHEN 0 THEN 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid
        WHEN 1 THEN 'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid
        WHEN 2 THEN 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid
    END,
    -- status: i%20=0 → DELETED, i%10=0 → INACTIVE, else ACTIVE
    CASE
        WHEN i % 20 = 0 THEN 'DELETED'::product_status
        WHEN i % 10 = 0 THEN 'INACTIVE'::product_status
        ELSE                  'ACTIVE'::product_status
    END,
    -- stock_quantity (5–204)
    5 + (i * 7) % 200,
    -- stock_reserved (0–9 for ACTIVE, 0 otherwise)
    CASE WHEN i % 10 != 0 AND i % 20 != 0 THEN (i * 3) % 10 ELSE 0 END,
    -- version
    0
FROM generate_series(1, 200) AS i;

-- ── Product images ────────────────────────────────────────────────────────────
-- Primary image for every product
INSERT INTO product_images (product_id, url, alt_text, sort_order)
SELECT
    id,
    'https://picsum.photos/seed/p' || id || '/600/400',
    name || ' — main view',
    0
FROM products;

-- Secondary image for every other product (even IDs)
INSERT INTO product_images (product_id, url, alt_text, sort_order)
SELECT
    id,
    'https://picsum.photos/seed/p' || id || 'b/600/400',
    name || ' — side view',
    1
FROM products
WHERE id % 2 = 0;
