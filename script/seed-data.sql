-- -- ============================================================
-- -- Seed Data for E-Commerce System
-- -- Idempotent: safe to run multiple times
-- -- Password for all users: "Password123!" (bcrypt cost 12)
-- -- ============================================================

-- -- ============================================================
-- -- ecommerce_users seed
-- -- ============================================================
-- \c ecommerce_users

-- INSERT INTO users (id, email, password_hash, role, is_locked, failed_login_attempts)
-- VALUES
--   ('a0000000-0000-0000-0000-000000000001', 'admin@example.com',    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'admin',    FALSE, 0),
--   ('a0000000-0000-0000-0000-000000000002', 'seller1@example.com',  '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'seller',   FALSE, 0),
--   ('a0000000-0000-0000-0000-000000000003', 'seller2@example.com',  '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'seller',   FALSE, 0),
--   ('a0000000-0000-0000-0000-000000000004', 'customer1@example.com','$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'customer', FALSE, 0),
--   ('a0000000-0000-0000-0000-000000000005', 'customer2@example.com','$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'customer', FALSE, 0),
--   ('a0000000-0000-0000-0000-000000000006', 'customer3@example.com','$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'customer', FALSE, 0),
--   ('a0000000-0000-0000-0000-000000000007', 'locked@example.com',   '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4J/HS.iK8i', 'customer', TRUE,  5)
-- ON CONFLICT (id) DO NOTHING;

-- INSERT INTO user_profiles (id, user_id, first_name, last_name, phone, avatar_url)
-- VALUES
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000001', 'Admin',   'User',    '+1-555-000-0001', NULL),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000002', 'Alice',   'Seller',  '+1-555-000-0002', 'https://i.pravatar.cc/150?u=seller1'),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000003', 'Bob',     'Seller',  '+1-555-000-0003', 'https://i.pravatar.cc/150?u=seller2'),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000004', 'Charlie', 'Customer','+1-555-000-0004', 'https://i.pravatar.cc/150?u=customer1'),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000005', 'Diana',   'Customer','+1-555-000-0005', 'https://i.pravatar.cc/150?u=customer2'),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000006', 'Eve',     'Customer','+1-555-000-0006', 'https://i.pravatar.cc/150?u=customer3'),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000007', 'Locked',  'User',    NULL,              NULL)
-- ON CONFLICT (user_id) DO NOTHING;

-- INSERT INTO user_addresses (id, user_id, address_line1, city, state, postal_code, country, is_default)
-- VALUES
--   -- customer1: 2 addresses
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000004', '123 Main St',     'New York',    'NY', '10001', 'USA', TRUE),
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000004', '456 Oak Ave',     'Brooklyn',    'NY', '11201', 'USA', FALSE),
--   -- customer2: 1 address
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000005', '789 Pine Rd',     'Los Angeles', 'CA', '90001', 'USA', TRUE),
--   -- customer3: 1 address
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000006', '321 Maple Blvd',  'Chicago',     'IL', '60601', 'USA', TRUE),
--   -- seller1: 1 address
--   (uuid_generate_v4(), 'a0000000-0000-0000-0000-000000000002', '1 Warehouse Way', 'Austin',      'TX', '78701', 'USA', TRUE)
-- ON CONFLICT DO NOTHING;


-- -- ============================================================
-- -- ecommerce_products seed
-- -- ============================================================
-- \c ecommerce_products

-- -- ---- Categories (2-level hierarchy) ----
-- INSERT INTO categories (id, name, slug, parent_id, sort_order)
-- VALUES
--   -- Root categories
--   (1,  'Electronics',      'electronics',       NULL, 1),
--   (2,  'Clothing',         'clothing',          NULL, 2),
--   (3,  'Home & Kitchen',   'home-kitchen',      NULL, 3),
--   (4,  'Books',            'books',             NULL, 4),
--   (5,  'Sports & Outdoors','sports-outdoors',   NULL, 5),
--   -- Electronics sub-categories
--   (6,  'Laptops',          'laptops',           1,    1),
--   (7,  'Smartphones',      'smartphones',       1,    2),
--   (8,  'Accessories',      'accessories',       1,    3),
--   (9,  'Audio',            'audio',             1,    4),
--   -- Clothing sub-categories
--   (10, 'Men''s Clothing',  'mens-clothing',     2,    1),
--   (11, 'Women''s Clothing','womens-clothing',   2,    2)
-- ON CONFLICT (id) DO NOTHING;

-- -- Reset sequence to continue after manual IDs
-- SELECT setval('categories_id_seq', (SELECT MAX(id) FROM categories));

-- -- ---- Products (100 products, varied categories) ----
-- INSERT INTO products (id, name, description, price, category_id, seller_id, status, stock_quantity, stock_reserved, version)
-- VALUES
--   -- Laptops (category 6)
--   (1,  'ProBook X1 Laptop',         'Intel i7, 16GB RAM, 512GB SSD, 15.6" FHD display. Ideal for developers.',            1299.99, 6, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   50,  2, 0),
--   (2,  'UltraSlim 14 Laptop',       'AMD Ryzen 5, 8GB RAM, 256GB SSD, fanless design, 12-hour battery.',                   799.99, 6, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   30,  0, 0),
--   (3,  'GameMaster Pro Laptop',     'Intel i9, RTX 4070, 32GB RAM, 1TB NVMe, 165Hz display. For serious gamers.',         2199.99, 6, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   15,  1, 0),
--   (4,  'BudgetBook 15',             'Celeron N4020, 4GB RAM, 128GB eMMC. Great for everyday tasks and students.',          299.99, 6, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  100,  5, 0),
--   (5,  'WorkStation Elite',         'Intel Xeon, 64GB ECC RAM, 2TB SSD, dual monitor support. Professional workstation.',  3499.99, 6, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',    8,  0, 0),
--   (6,  'ThinPad Carbon',            'Carbon fiber body, Intel i5, 8GB RAM, 256GB SSD, 14" IPS. Ultra-portable.',           999.99, 6, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   40,  3, 0),
--   (7,  'CreatorBook 16',            '4K OLED display, Intel i7, 32GB RAM, 1TB SSD, NVIDIA RTX 3060. For creators.',       1799.99, 6, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   20,  0, 0),
--   (8,  'EduPad Laptop',             'Rugged design for schools, 11.6" display, 4GB RAM, 64GB storage, water-resistant.',   249.99, 6, 'a0000000-0000-0000-0000-000000000003', 'INACTIVE',  0,  0, 0),

--   -- Smartphones (category 7)
--   (9,  'Nova X Pro',                '6.7" AMOLED, 200MP camera, 5000mAh, 120Hz, Snapdragon 8 Gen 2.',                      999.99, 7, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   75,  4, 0),
--   (10, 'Nova Lite',                 '6.1" LCD, 50MP camera, 4000mAh, Helio G99. Budget flagship killer.',                  349.99, 7, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  120,  8, 0),
--   (11, 'PixelMax 7',                '6.3" OLED, Tensor G3 chip, 50MP camera, 7 years of OS updates.',                      899.99, 7, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   60,  2, 0),
--   (12, 'Fold Ultra',                '7.6" foldable OLED + 6.2" cover, Snapdragon 8 Gen 2, 12GB RAM.',                     1799.99, 7, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   25,  0, 0),
--   (13, 'BudgetPhone 5G',            '6.5" IPS, 48MP, MediaTek Dimensity 700, 5G capable, 5000mAh.',                        199.99, 7, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  200, 10, 0),
--   (14, 'SecurePhone X',             'Privacy-focused, encrypted storage, de-Googled Android, no tracking.',                649.99, 7, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   18,  0, 0),
--   (15, 'KidsPhone Safe',            'Parental controls built-in, rugged, 4" display, GPS tracking, 2-year durability.',    149.99, 7, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   45,  1, 0),

--   -- Accessories (category 8)
--   (16, 'MechKey Pro Keyboard',      'Cherry MX Blue switches, TKL layout, PBT keycaps, RGB backlight, USB-C.',              129.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   80,  6, 0),
--   (17, 'Wireless Ergonomic Mouse',  'Ergonomic design, 2.4GHz + Bluetooth, 8 programmable buttons, 90-day battery.',         59.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  150,  0, 0),
--   (18, '27" 4K Monitor',            'IPS, 144Hz, HDR400, USB-C 65W, sRGB 99%, height adjustable stand.',                   449.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   35,  2, 0),
--   (19, 'USB-C Hub 12-in-1',         'HDMI 4K@60Hz, 3x USB-A, SD/MicroSD, Ethernet, 100W PD pass-through.',                  49.99, 8, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  200,  0, 0),
--   (20, 'Webcam 4K Ultra',           '4K 30fps, Sony sensor, built-in stereo mic, auto-focus, privacy cover.',               119.99, 8, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   60,  4, 0),
--   (21, 'Laptop Stand Aluminum',     'Adjustable height 6-levels, foldable, fits 10-17" laptops, 10kg load capacity.',        39.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  300,  0, 0),
--   (22, '65W GaN Charger',           '4-port GaN charger, 65W total, 2x USB-C PD + 2x USB-A, compact travel size.',          45.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  180,  5, 0),
--   (23, 'Screen Cleaning Kit',       'Microfiber cloth + spray solution + storage bag. Safe for all screens.',                12.99, 8, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  500,  0, 0),

--   -- Audio (category 9)
--   (24, 'StudioPods Pro',            'Active noise cancellation, 30hr battery, LDAC, Bluetooth 5.3, IPX4.',                  249.99, 9, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   90,  7, 0),
--   (25, 'BassBoost 500 Headphones',  'Over-ear, 40mm drivers, foldable, 3.5mm + USB-C, 32hr battery, ANC.',                  179.99, 9, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   55,  3, 0),
--   (26, 'Desk Speaker 2.1',          '60W RMS, subwoofer, Bluetooth 5.0, optical in, 3.5mm, RGB ambient light.',             199.99, 9, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   30,  0, 0),
--   (27, 'Condenser Microphone USB',  'Cardioid pattern, 24-bit/96kHz, built-in headphone monitor, metal body.',               89.99, 9, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   70,  2, 0),
--   (28, 'True Wireless Earbuds Lite','6mm drivers, 5hr + 20hr case, IPX5, touch controls, fast pairing.',                    39.99, 9, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  250,  15, 0),
--   (29, 'Gaming Headset 7.1',        'Virtual 7.1 surround, 50mm neodymium drivers, noise-cancelling boom mic, LED.',         79.99, 9, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   85,  4, 0),

--   -- Men's Clothing (category 10)
--   (30, 'Classic Oxford Shirt',      '100% cotton, regular fit, available in white/blue/black, wrinkle-resistant.',            49.99, 10,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  200,  0, 0),
--   (31, 'Slim Fit Chinos',           'Stretch cotton blend, 5-pocket, available in khaki/navy/olive, tapered leg.',            59.99, 10,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  150,  0, 0),
--   (32, 'Performance Polo',          'Moisture-wicking, quick-dry, UV protection UPF 50+. Perfect for golf and casual.',       34.99, 10,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  180,  5, 0),
--   (33, 'Merino Wool Sweater',       '100% merino wool, crew neck, slim fit. Soft, warm, itch-free.',                          89.99, 10,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   75,  2, 0),
--   (34, 'Cargo Shorts',              'Durable ripstop fabric, 6 pockets, elastic waistband, quick-dry.',                       39.99, 10,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  120,  0, 0),
--   (35, 'Puffer Jacket Men',         'Recycled fill, water-resistant, packable, 3 colors. Great for travel.',                  119.99, 10,'a0000000-0000-0000-0000-000000000003','ACTIVE',   60,  1, 0),

--   -- Women's Clothing (category 11)
--   (36, 'Floral Summer Dress',       'Lightweight chiffon, floral print, midi length, spaghetti straps. S-XL.',                44.99, 11,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  100,  0, 0),
--   (37, 'High-Rise Yoga Pants',      '4-way stretch, moisture-wicking, 2 pockets, 7/8 length. XS-XXL.',                       49.99, 11,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  180,  8, 0),
--   (38, 'Cashmere Blend Cardigan',   '70% cashmere/30% wool, open front, longline, available in 4 colors.',                   129.99, 11,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   55,  0, 0),
--   (39, 'Linen Wide-Leg Pants',      '100% linen, elastic waist, breathable, great for summer office wear.',                   64.99, 11,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   90,  3, 0),
--   (40, 'Puffer Vest Women',         'Lightweight, quilted, zip front, 2 zip pockets. Packable to palm size.',                  69.99, 11,'a0000000-0000-0000-0000-000000000003','ACTIVE',   70,  0, 0),

--   -- Home & Kitchen (category 3)
--   (41, 'Coffee Maker 12-Cup',       'Programmable, built-in grinder, thermal carafe, 24hr auto-start, pause & pour.',        129.99, 3, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   45,  2, 0),
--   (42, 'Instant Pot 7-in-1',        '6Qt, pressure cooker, slow cooker, rice cooker, steamer, sauté, yogurt maker.',          99.99, 3, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   80,  5, 0),
--   (43, 'Air Fryer XL 5.8Qt',        'Digital display, 8 presets, dishwasher-safe basket, auto shutoff.',                      79.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   65,  3, 0),
--   (44, 'Chef''s Knife 8"',          'German steel, full tang, ergonomic handle, razor-sharp edge, includes sheath.',           69.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  120,  0, 0),
--   (45, 'Cast Iron Skillet 12"',     'Pre-seasoned, oven safe to 500°F, compatible with all cooktops including induction.',     49.99, 3, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  100,  0, 0),
--   (46, 'Blender Pro 1200W',         'Variable speed, pulse, stainless blades, 64oz jar, self-cleaning, BPA-free.',            149.99, 3, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   40,  1, 0),
--   (47, 'Bamboo Cutting Board Set',  '3-piece set (S/M/L), juice groove, anti-slip feet, easy to clean, eco-friendly.',        34.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  200,  0, 0),
--   (48, 'Stainless Steel Cookware Set','10-piece set, tri-ply construction, oven safe, dishwasher safe, induction compatible.', 299.99, 3,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   25,  0, 0),
--   (49, 'Digital Kitchen Scale',     '11lb/5kg, 0.1g precision, tare function, 4 units, backlit LCD, auto-off.',               19.99, 3, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  300,  0, 0),
--   (50, 'Espresso Machine Manual',   'Pump pressure 15 bar, steam wand, 1.8L tank, stainless body, cup warmer.',               349.99, 3,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   20,  0, 0),

--   -- Books (category 4)
--   (51, 'Clean Code',                'Robert C. Martin. Write readable, maintainable, and clean code. Essential for devs.',     39.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  500,  20, 0),
--   (52, 'Designing Data-Intensive Applications', 'Martin Kleppmann. Distributed systems, databases, and data systems.',         54.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  400,  15, 0),
--   (53, 'The Go Programming Language','Alan Donovan & Brian Kernighan. The authoritative guide to Go.',                          44.99, 4, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  300,   8, 0),
--   (54, 'Spring Boot in Action',     'Craig Walls. Build Spring Boot applications from scratch with practical examples.',        49.99, 4, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  250,   5, 0),
--   (55, 'System Design Interview',   'Alex Xu. Volume 1 & 2 bundle. Top FAANG interview preparation.',                          59.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  600,  30, 0),
--   (56, 'Kafka: The Definitive Guide','Neha Narkhede et al. Real-time data streaming and event-driven architecture.',            49.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  200,   6, 0),
--   (57, 'Redis in Action',           'Josiah Carlson. Practical Redis for real-world applications.',                             34.99, 4, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  180,   4, 0),
--   (58, 'Docker Deep Dive',          'Nigel Poulton. Containers, images, networking, volumes, compose, Swarm.',                  29.99, 4, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  220,   3, 0),

--   -- Sports & Outdoors (category 5)
--   (59, 'Yoga Mat Premium 6mm',      'Non-slip, eco-friendly TPE, alignment lines, carry strap, 72"x24".',                      44.99, 5, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  150,   5, 0),
--   (60, 'Resistance Bands Set (5)',  'Loop bands, 5 resistance levels, latex-free TPE, workout guide included.',                 24.99, 5, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  300,   0, 0),
--   (61, 'Adjustable Dumbbell 50lb',  'Select 5-50lbs in 2.5lb increments, replaces 15 dumbbells, compact storage.',             299.99, 5,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   30,   1, 0),
--   (62, 'Running Shoes Pro',         'Carbon fiber plate, foam midsole, breathable mesh, drop 6mm, unisex sizing.',             189.99, 5,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   80,   3, 0),
--   (63, 'Hiking Backpack 45L',       'Waterproof, ventilated back panel, hydration sleeve, rain cover, 45L, padded straps.',    129.99, 5,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   55,   0, 0),
--   (64, 'Jump Rope Steel Cable',     'Speed rope, ball bearings, adjustable length, foam handles, counter.',                     14.99, 5,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  400,   0, 0),
--   (65, 'Foam Roller Deep Tissue',   'High-density EVA foam, textured surface, 18", supports up to 300lbs.',                    29.99, 5, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  250,   0, 0),
--   (66, 'Cycling Helmet MIPS',       'MIPS protection, 20 vents, dial fit system, rear LED, CE EN1078 certified.',               89.99, 5,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   60,   2, 0),

--   -- More Electronics - Accessories (category 8)
--   (67, 'Thunderbolt 4 Dock',        '11 ports: 4K@60Hz, 2.5G Ethernet, 60W PD, 3x USB-A, SD card, audio.',                   199.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   40,   1, 0),
--   (68, 'Portable SSD 1TB',          '1TB, USB 3.2 Gen 2, 1000MB/s read, drop-proof, pocket-sized.',                           109.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   90,   4, 0),
--   (69, 'Smart Power Strip',         '6 outlets + 4 USB, app control, energy monitoring, surge protection, scheduling.',         49.99, 8, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  120,   0, 0),
--   (70, 'Cable Management Kit',      '150-piece kit: velcro straps, cable clips, sleeves, labels. Clean desk setup.',            19.99, 8, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  350,   0, 0),

--   -- More Smartphones (category 7)
--   (71, 'SmartWatch Ultra',          '1.9" OLED, GPS, ECG, SpO2, 60hr battery, IP68, 100+ sport modes.',                       299.99, 7, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   65,   3, 0),
--   (72, 'Fitness Tracker Band 6',    '1.4" color display, heart rate, SpO2, sleep tracking, 14-day battery, IP68.',              49.99, 7, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  200,  10, 0),

--   -- More Home & Kitchen (category 3)
--   (73, 'Robot Vacuum & Mop Combo',  'LiDAR mapping, auto-empty dock, 5000Pa suction, mop attachment, app control.',            499.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   20,   0, 0),
--   (74, 'Air Purifier True HEPA',    'HEPA + carbon filter, covers 500sq ft, CADR 260, auto mode, WiFi, quiet 24dB.',           179.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   35,   1, 0),
--   (75, 'Stand Mixer 5Qt',           '10-speed, dough hook + flat beater + wire whip, splash guard, tilt-head.',               349.99, 3, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   18,   0, 0),

--   -- More Books (category 4)
--   (76, 'Kubernetes in Action',      'Marko Luksa. Deploy, manage, and scale containerized applications.',                       54.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  180,   4, 0),
--   (77, 'PostgreSQL: Up & Running',  'Regina Obe & Leo Hsu. Practical guide to PostgreSQL administration and queries.',          39.99, 4, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  150,   2, 0),
--   (78, 'Microservices Patterns',    'Chris Richardson. Patterns for decomposing monoliths, sagas, CQRS.',                      54.99, 4, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  220,   8, 0),

--   -- More Sports (category 5)
--   (79, 'Pull-Up Bar Doorframe',     'No screws, fits 24-32" doors, 300lb capacity, multi-grip, foam padding.',                  34.99, 5, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  180,   0, 0),
--   (80, 'Whey Protein Powder 5lb',   '25g protein per serving, 2g fat, 3g carbs. Chocolate Fudge / Vanilla. Informed Sport.',    69.99, 5,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  300,  12, 0),

--   -- More Audio (category 9)
--   (81, 'Soundbar 2.1 with Sub',     '120W, Bluetooth 5.0, HDMI ARC, optical, DSP modes, wall mountable.',                      199.99, 9, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   30,   0, 0),
--   (82, 'Vinyl Record Player',       '3-speed, built-in preamp, Bluetooth out, RCA out, dust cover, belt drive.',               129.99, 9, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   25,   1, 0),

--   -- More Men's Clothing (category 10)
--   (83, 'Athletic Track Pants',      '88% polyester, jogger fit, zip pockets, reflective, quick-dry.',                           44.99, 10,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  130,   4, 0),
--   (84, 'Denim Jeans Slim Fit',      'Stretch denim, 5-pocket, slim taper, available in black/dark blue/light wash.',            69.99, 10,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  160,   0, 0),

--   -- More Women's Clothing (category 11)
--   (85, 'Sleeveless Blouse Silk',    '100% mulberry silk, V-neck, 5 colors, machine washable, relaxed fit.',                     79.99, 11,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   80,   0, 0),
--   (86, 'Athletic Sports Bra',       'Medium support, removable pads, moisture-wicking, strappy back design.',                   29.99, 11,'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  200,   6, 0),

--   -- More Laptops (category 6)
--   (87, 'MacBook-style Laptop 13"',  'M2-class ARM chip, 8GB RAM, 256GB, fanless, 18hr battery, Thunderbolt 4.',               1099.99, 6,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   22,   0, 0),
--   (88, 'Chromebook 14 HD',          'MediaTek MT8183, 4GB, 32GB eMMC, Chrome OS, 12hr battery, backlit keyboard.',             279.99, 6,'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   60,   0, 0),

--   -- More Accessories (category 8)
--   (89, 'Wireless Charging Pad 15W', 'Qi2 15W, compatible with all Qi devices, LED indicator, anti-slip base.',                  29.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  400,   0, 0),
--   (90, 'Desk Mat XL (90x40cm)',     'Non-slip base, stitched edges, waterproof surface, black/grey.',                           24.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  250,   0, 0),

--   -- More Home (category 3)
--   (91, 'Electric Kettle 1.7L',      '1500W, rapid boil, keep warm 30min, 360° base, auto-off, boil-dry protection.',            39.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  150,   3, 0),
--   (92, 'Toaster Oven 25L',          '1800W, 12 cooking functions, convection, rotisserie, digital display, timer.',             89.99, 3, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',   40,   1, 0),

--   -- More Sports (category 5)
--   (93, 'Hydration Pack 2L',         '2L bladder, 10L storage, breathable back, hiking poles attachment, rain cover.',           59.99, 5, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   90,   0, 0),
--   (94, 'Knee Sleeves (pair)',        'Neoprene, 7mm compression, anti-slip, sizes S-XXL. For lifting and running.',              27.99, 5, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  200,   0, 0),

--   -- More Books (category 4)
--   (95, 'Domain-Driven Design',      'Eric Evans. Blue book. The foundation of DDD, bounded contexts, aggregates.',              54.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  150,   5, 0),
--   (96, 'The Pragmatic Programmer',  'Hunt & Thomas. 20th Anniversary Edition. Tips for every serious developer.',               44.99, 4, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',  250,   3, 0),

--   -- More Audio (category 9)
--   (97, 'Portable Bluetooth Speaker','360° sound, IPX7, 24hr battery, built-in mic, USB-C charge, party mode.',                  59.99, 9, 'a0000000-0000-0000-0000-000000000003', 'ACTIVE',  180,   5, 0),

--   -- More Men's/Women's Clothing
--   (98, 'Waterproof Rain Jacket',    'Seam-sealed, 3-layer, packable, adjustable hood, pit zips, reflective.',                   149.99, 10,'a0000000-0000-0000-0000-000000000003','ACTIVE',   70,   0, 0),
--   (99, 'Thermal Base Layer Set',    'Top + bottom, 200g merino wool, temperature regulation, machine wash.',                     99.99, 11,'a0000000-0000-0000-0000-000000000002','ACTIVE',   60,   2, 0),

--   -- Low-stock item for testing concurrency
--   (100,'Limited Edition Keyboard',  'Artisan keycaps, brass plate, lubed linear switches. Only 10 units available!',           399.99, 8, 'a0000000-0000-0000-0000-000000000002', 'ACTIVE',   10,   0, 0)
-- ON CONFLICT (id) DO NOTHING;

-- -- Reset product sequence
-- SELECT setval('products_id_seq', (SELECT MAX(id) FROM products));

-- -- ---- Product Images ----
-- INSERT INTO product_images (product_id, url, alt_text, sort_order)
-- VALUES
--   (1,  'https://placehold.co/600x400?text=ProBook+X1',           'ProBook X1 Laptop front view',         1),
--   (1,  'https://placehold.co/600x400?text=ProBook+X1+Side',      'ProBook X1 Laptop side view',          2),
--   (9,  'https://placehold.co/600x400?text=Nova+X+Pro',           'Nova X Pro smartphone',                1),
--   (16, 'https://placehold.co/600x400?text=MechKey+Pro',          'MechKey Pro Keyboard',                 1),
--   (24, 'https://placehold.co/600x400?text=StudioPods+Pro',       'StudioPods Pro earbuds in case',       1),
--   (41, 'https://placehold.co/600x400?text=Coffee+Maker',         'Coffee Maker 12-Cup',                  1),
--   (42, 'https://placehold.co/600x400?text=Instant+Pot',          'Instant Pot 7-in-1',                   1),
--   (51, 'https://placehold.co/600x400?text=Clean+Code',           'Clean Code book cover',                1),
--   (52, 'https://placehold.co/600x400?text=DDIA',                 'Designing Data-Intensive Applications',1),
--   (55, 'https://placehold.co/600x400?text=System+Design',        'System Design Interview book',         1),
--   (61, 'https://placehold.co/600x400?text=Dumbbell',             'Adjustable Dumbbell set',              1),
--   (100,'https://placehold.co/600x400?text=Limited+Keyboard',     'Limited Edition Keyboard top view',    1),
--   (100,'https://placehold.co/600x400?text=Limited+Keyboard+Side','Limited Edition Keyboard side view',   2)
-- ON CONFLICT DO NOTHING;

-- -- ---- Stock Movements (initial stock IN) ----
-- INSERT INTO stock_movements (product_id, type, quantity, reference_id, reason)
-- SELECT id, 'IN', stock_quantity, 'SEED-INIT', 'Initial inventory load'
-- FROM products
-- WHERE stock_quantity > 0
-- ON CONFLICT DO NOTHING;


-- -- ============================================================
-- -- ecommerce_carts seed
-- -- ============================================================
-- \c ecommerce_carts

-- INSERT INTO carts (id, user_id, status, expires_at)
-- VALUES
--   -- customer1: active cart with items
--   ('c0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000004', 'ACTIVE',       NOW() + INTERVAL '30 minutes'),
--   -- customer2: active empty cart
--   ('c0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000005', 'ACTIVE',       NOW() + INTERVAL '30 minutes'),
--   -- customer3: checked out cart (historical)
--   ('c0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000006', 'CHECKED_OUT',  NOW() - INTERVAL '2 days'),
--   -- customer1: older abandoned cart
--   ('c0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000004', 'ABANDONED',    NOW() - INTERVAL '7 days')
-- ON CONFLICT (id) DO NOTHING;

-- INSERT INTO cart_items (id, cart_id, product_id, product_name, quantity, unit_price)
-- VALUES
--   -- customer1's active cart (cart 001): laptop + wireless mouse + book
--   (uuid_generate_v4(), 'c0000000-0000-0000-0000-000000000001', 1,  'ProBook X1 Laptop',               1, 1299.99),
--   (uuid_generate_v4(), 'c0000000-0000-0000-0000-000000000001', 17, 'Wireless Ergonomic Mouse',         1,   59.99),
--   (uuid_generate_v4(), 'c0000000-0000-0000-0000-000000000001', 55, 'System Design Interview',          2,   59.99),
--   -- customer3's checked-out cart (cart 003): used for order seed
--   (uuid_generate_v4(), 'c0000000-0000-0000-0000-000000000003', 24, 'StudioPods Pro',                   1,  249.99),
--   (uuid_generate_v4(), 'c0000000-0000-0000-0000-000000000003', 28, 'True Wireless Earbuds Lite',       2,   39.99)
-- ON CONFLICT DO NOTHING;


-- -- ============================================================
-- -- ecommerce_orders seed
-- -- ============================================================
-- \c ecommerce_orders

-- -- ---- Orders ----
-- INSERT INTO orders (id, user_id, cart_id, total_amount, status, shipping_address)
-- VALUES
--   -- customer3: DELIVERED order
--   (
--     'e0000000-0000-0000-0000-000000000001',
--     'a0000000-0000-0000-0000-000000000006',
--     'c0000000-0000-0000-0000-000000000003',
--     329.97,
--     'DELIVERED',
--     '{"street":"321 Maple Blvd","city":"Chicago","state":"IL","zip":"60601","country":"USA"}'
--   ),
--   -- customer2: CONFIRMED order (payment succeeded, not yet shipped)
--   (
--     'e0000000-0000-0000-0000-000000000002',
--     'a0000000-0000-0000-0000-000000000005',
--     NULL,
--     99.99,
--     'CONFIRMED',
--     '{"street":"789 Pine Rd","city":"Los Angeles","state":"CA","zip":"90001","country":"USA"}'
--   ),
--   -- customer1: PENDING order (awaiting payment)
--   (
--     'e0000000-0000-0000-0000-000000000003',
--     'a0000000-0000-0000-0000-000000000004',
--     NULL,
--     1419.97,
--     'PENDING',
--     '{"street":"123 Main St","city":"New York","state":"NY","zip":"10001","country":"USA"}'
--   ),
--   -- customer1: CANCELLED order (payment failed)
--   (
--     'e0000000-0000-0000-0000-000000000004',
--     'a0000000-0000-0000-0000-000000000004',
--     NULL,
--     179.99,
--     'CANCELLED',
--     '{"street":"123 Main St","city":"New York","state":"NY","zip":"10001","country":"USA"}'
--   ),
--   -- customer2: SHIPPED order
--   (
--     'e0000000-0000-0000-0000-000000000005',
--     'a0000000-0000-0000-0000-000000000005',
--     NULL,
--     54.99,
--     'SHIPPED',
--     '{"street":"789 Pine Rd","city":"Los Angeles","state":"CA","zip":"90001","country":"USA"}'
--   )
-- ON CONFLICT (id) DO NOTHING;

-- -- ---- Order Items ----
-- INSERT INTO order_items (id, order_id, product_id, product_name, quantity, unit_price)
-- VALUES
--   -- Order 001 (DELIVERED): StudioPods Pro + True Wireless Earbuds x2
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000001', 24, 'StudioPods Pro',            1, 249.99),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000001', 28, 'True Wireless Earbuds Lite',2,  39.99),
--   -- Order 002 (CONFIRMED): Instant Pot
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000002', 42, 'Instant Pot 7-in-1',        1,  99.99),
--   -- Order 003 (PENDING): ProBook X1 + Wireless Mouse + System Design x1
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000003', 1,  'ProBook X1 Laptop',         1, 1299.99),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000003', 17, 'Wireless Ergonomic Mouse',  1,   59.99),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000003', 55, 'System Design Interview',   1,   59.99),
--   -- Order 004 (CANCELLED): BassBoost 500 Headphones
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000004', 25, 'BassBoost 500 Headphones',  1, 179.99),
--   -- Order 005 (SHIPPED): Spring Boot in Action
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000005', 54, 'Spring Boot in Action',     1,  54.99)
-- ON CONFLICT DO NOTHING;

-- -- ---- Order Status History ----
-- INSERT INTO order_status_history (order_id, old_status, new_status, reason, changed_by)
-- VALUES
--   -- Order 001: full DELIVERED journey
--   ('e0000000-0000-0000-0000-000000000001', NULL,        'PENDING',   'Order placed',               'system'),
--   ('e0000000-0000-0000-0000-000000000001', 'PENDING',   'CONFIRMED', 'Payment succeeded',          'kafka-consumer'),
--   ('e0000000-0000-0000-0000-000000000001', 'CONFIRMED', 'SHIPPED',   'Shipped via FedEx',          'seller'),
--   ('e0000000-0000-0000-0000-000000000001', 'SHIPPED',   'DELIVERED', 'Delivered to customer',      'system'),
--   -- Order 002: CONFIRMED
--   ('e0000000-0000-0000-0000-000000000002', NULL,        'PENDING',   'Order placed',               'system'),
--   ('e0000000-0000-0000-0000-000000000002', 'PENDING',   'CONFIRMED', 'Payment succeeded',          'kafka-consumer'),
--   -- Order 003: PENDING
--   ('e0000000-0000-0000-0000-000000000003', NULL,        'PENDING',   'Order placed',               'system'),
--   -- Order 004: CANCELLED
--   ('e0000000-0000-0000-0000-000000000004', NULL,        'PENDING',   'Order placed',               'system'),
--   ('e0000000-0000-0000-0000-000000000004', 'PENDING',   'CANCELLED', 'Payment failed: card declined','kafka-consumer'),
--   -- Order 005: SHIPPED
--   ('e0000000-0000-0000-0000-000000000005', NULL,        'PENDING',   'Order placed',               'system'),
--   ('e0000000-0000-0000-0000-000000000005', 'PENDING',   'CONFIRMED', 'Payment succeeded',          'kafka-consumer'),
--   ('e0000000-0000-0000-0000-000000000005', 'CONFIRMED', 'SHIPPED',   'Shipped via UPS #1Z999',     'seller')
-- ON CONFLICT DO NOTHING;

-- -- ---- Notifications ----
-- INSERT INTO notifications (id, order_id, user_id, type, channel, subject, body, status, sent_at)
-- VALUES
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000006', 'EMAIL', 'customer3@example.com', 'Order #o0000000-0001 Confirmed',  'Your order has been confirmed.',        'SENT', NOW() - INTERVAL '2 days'),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000006', 'EMAIL', 'customer3@example.com', 'Order #o0000000-0001 Shipped',    'Your order is on its way!',             'SENT', NOW() - INTERVAL '1 day'),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000006', 'EMAIL', 'customer3@example.com', 'Order #o0000000-0001 Delivered',  'Your order has been delivered.',        'SENT', NOW() - INTERVAL '12 hours'),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000005', 'EMAIL', 'customer2@example.com', 'Order #o0000000-0002 Confirmed',  'Your order has been confirmed.',        'SENT', NOW() - INTERVAL '1 day'),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000004', 'EMAIL', 'customer1@example.com', 'Order #o0000000-0004 Cancelled',  'Your payment was declined. Order cancelled.', 'SENT', NOW() - INTERVAL '3 days'),
--   (uuid_generate_v4(), 'e0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000005', 'EMAIL', 'customer2@example.com', 'Order #o0000000-0005 Shipped',    'Your order shipped via UPS #1Z999.',   'SENT', NOW() - INTERVAL '6 hours')
-- ON CONFLICT DO NOTHING;


-- -- ============================================================
-- -- ecommerce_payments seed
-- -- ============================================================
-- \c ecommerce_payments

-- INSERT INTO payments (id, order_id, user_id, amount, currency, status, method, idempotency_key, gateway_reference)
-- VALUES
--   -- Order 001: COMPLETED (DELIVERED order)
--   (
--     'f0000000-0000-0000-0000-000000000001',
--     'e0000000-0000-0000-0000-000000000001',
--     'a0000000-0000-0000-0000-000000000006',
--     329.97, 'USD', 'COMPLETED', 'MOCK_CARD',
--     'idempotency-order-001-customer3',
--     'GW-REF-7A3F9C2B'
--   ),
--   -- Order 002: COMPLETED (CONFIRMED order)
--   (
--     'f0000000-0000-0000-0000-000000000002',
--     'e0000000-0000-0000-0000-000000000002',
--     'a0000000-0000-0000-0000-000000000005',
--     99.99, 'USD', 'COMPLETED', 'MOCK_WALLET',
--     'idempotency-order-002-customer2',
--     'GW-REF-1D8E4A6F'
--   ),
--   -- Order 003: PENDING (waiting for payment processing)
--   (
--     'f0000000-0000-0000-0000-000000000003',
--     'e0000000-0000-0000-0000-000000000003',
--     'a0000000-0000-0000-0000-000000000004',
--     1419.97, 'USD', 'PENDING', 'MOCK_CARD',
--     'idempotency-order-003-customer1',
--     NULL
--   ),
--   -- Order 004: FAILED (CANCELLED order)
--   (
--     'f0000000-0000-0000-0000-000000000004',
--     'e0000000-0000-0000-0000-000000000004',
--     'a0000000-0000-0000-0000-000000000004',
--     179.99, 'USD', 'FAILED', 'MOCK_CARD',
--     'idempotency-order-004-customer1',
--     NULL
--   ),
--   -- Order 005: COMPLETED (SHIPPED order)
--   (
--     'f0000000-0000-0000-0000-000000000005',
--     'e0000000-0000-0000-0000-000000000005',
--     'a0000000-0000-0000-0000-000000000005',
--     54.99, 'USD', 'COMPLETED', 'MOCK_CARD',
--     'idempotency-order-005-customer2',
--     'GW-REF-5B2C8D4E'
--   )
-- ON CONFLICT (id) DO NOTHING;

-- INSERT INTO payment_history (payment_id, old_status, new_status, reason)
-- VALUES
--   -- Payment 001: PENDING → COMPLETED
--   ('f0000000-0000-0000-0000-000000000001', NULL,        'PENDING',   'Payment initiated'),
--   ('f0000000-0000-0000-0000-000000000001', 'PENDING',   'COMPLETED', 'Mock gateway approved: GW-REF-7A3F9C2B'),
--   -- Payment 002: PENDING → COMPLETED
--   ('f0000000-0000-0000-0000-000000000002', NULL,        'PENDING',   'Payment initiated'),
--   ('f0000000-0000-0000-0000-000000000002', 'PENDING',   'COMPLETED', 'Mock gateway approved: GW-REF-1D8E4A6F'),
--   -- Payment 003: PENDING (still in progress)
--   ('f0000000-0000-0000-0000-000000000003', NULL,        'PENDING',   'Payment initiated'),
--   -- Payment 004: PENDING → FAILED
--   ('f0000000-0000-0000-0000-000000000004', NULL,        'PENDING',   'Payment initiated'),
--   ('f0000000-0000-0000-0000-000000000004', 'PENDING',   'FAILED',    'Mock gateway declined: card number 4000000000000002'),
--   -- Payment 005: PENDING → COMPLETED
--   ('f0000000-0000-0000-0000-000000000005', NULL,        'PENDING',   'Payment initiated'),
--   ('f0000000-0000-0000-0000-000000000005', 'PENDING',   'COMPLETED', 'Mock gateway approved: GW-REF-5B2C8D4E')
-- ON CONFLICT DO NOTHING;
