#!/usr/bin/env python3
"""
Wipe all existing product/seller/cart/order/payment data and seed fresh:
  10 specialist sellers, 10 root categories (41 leaf), 10,000 products, 20,000 images.

Usage:
  python3 script/seed_products.py                   # full clean + reseed
  python3 script/seed_products.py --target 5000     # fewer products
  python3 script/seed_products.py --skip-clean      # skip cleanup (additive)
  python3 script/seed_products.py --dry-run         # print only, no writes

Prerequisites:
  pip install "psycopg[binary]" bcrypt

Env vars (defaults match docker-compose localhost exposure):
  USERS_DB_HOST/PORT/NAME/USER/PASSWORD
  PRODUCTS_DB_HOST/PORT/NAME/USER/PASSWORD
  CARTS_DB_HOST/PORT/NAME/USER/PASSWORD
  ORDERS_DB_HOST/PORT/NAME/USER/PASSWORD
  PAYMENTS_DB_HOST/PORT/NAME/USER/PASSWORD
"""
import argparse
import itertools
import os
import re
import random
import time

import bcrypt
import psycopg


# ── DB connection strings ──────────────────────────────────────────────────────

def _dsn(prefix: str, dbname: str) -> str:
    return (
        f"host={os.getenv(f'{prefix}_DB_HOST', 'localhost')} "
        f"port={os.getenv(f'{prefix}_DB_PORT', '5432')} "
        f"dbname={os.getenv(f'{prefix}_DB_NAME', dbname)} "
        f"user={os.getenv(f'{prefix}_DB_USER', 'postgres')} "
        f"password={os.getenv(f'{prefix}_DB_PASSWORD', 'postgres')}"
    )


USERS_DSN    = _dsn("USERS",    "ecommerce_users")
PRODUCTS_DSN = _dsn("PRODUCTS", "ecommerce_products")
CARTS_DSN    = _dsn("CARTS",    "ecommerce_carts")
ORDERS_DSN   = _dsn("ORDERS",   "ecommerce_orders")
PAYMENTS_DSN = _dsn("PAYMENTS", "ecommerce_payments")

SELLER_PASSWORD = "Password123!"

# These 3 sample-user IDs are NEVER deleted by cleanup.
PROTECTED_USER_IDS = (
    "00000000-0000-0000-0000-000000000001",  # admin@example.com
    "00000000-0000-0000-0000-000000000002",  # customer@example.com
    "00000000-0000-0000-0000-000000000003",  # seller@example.com
)

# ── Sellers ────────────────────────────────────────────────────────────────────
# 10 sellers, each exclusively owns one root category.
# First 3 UUIDs match V2 Flyway seed placeholders (safe since cleanup wipes those products).

SELLERS = [
    {"id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller01@techhub.com",        "first_name": "Alex",    "last_name": "Chen",    "store": "TechHub"},
    {"id": "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller02@fashionforward.com", "first_name": "Maria",   "last_name": "Santos",  "store": "FashionForward"},
    {"id": "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller03@homeessentials.com", "first_name": "James",   "last_name": "Wilson",  "store": "HomeEssentials"},
    {"id": "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller04@bookverse.com",      "first_name": "Priya",   "last_name": "Patel",   "store": "BookVerse"},
    {"id": "e0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller05@sportspeak.com",     "first_name": "Sophie",  "last_name": "Martin",  "store": "SportsPeak"},
    {"id": "f0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller06@beautybox.com",      "first_name": "David",   "last_name": "Kim",     "store": "BeautyBox"},
    {"id": "a1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller07@babybliss.com",      "first_name": "Carlos",  "last_name": "Gomez",   "store": "BabyBliss"},
    {"id": "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller08@petparadise.com",    "first_name": "Emma",    "last_name": "Johnson", "store": "PetParadise"},
    {"id": "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller09@foodcraft.com",      "first_name": "Liam",    "last_name": "Brown",   "store": "FoodCraft"},
    {"id": "d1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller10@musiczone.com",      "first_name": "Aisha",   "last_name": "Okafor",  "store": "MusicZone"},
]

# ── Category taxonomy ──────────────────────────────────────────────────────────
# Each root entry has a "seller_email" that maps to the exclusive seller for that root.
# Each leaf has: name, slug, price (min, max), nouns, adjs, brands.

TAXONOMY = [
    # ── Electronics ──────────────────────────────────────────────────────────
    {
        "root": "Electronics", "root_slug": "electronics",
        "seller_email": "seller01@techhub.com",
        "leaves": [
            {"name": "Laptops & Computers", "slug": "laptops-computers", "price": (349.99, 2499.99),
             "nouns": ["Laptop", "Ultrabook", "Gaming Laptop", "Chromebook", "Mini PC", "All-in-One PC"],
             "adjs": ["16GB RAM", "512GB SSD", "Backlit Keyboard", "4K Display", "Thin & Light"],
             "brands": ["TechPro", "SwiftBook", "CoreX", "PixelEdge", "NovaTech"]},
            {"name": "Smartphones & Tablets", "slug": "smartphones-tablets", "price": (149.99, 1299.99),
             "nouns": ["Smartphone", "Android Phone", "Tablet", "iPad Alternative", "Flagship Phone", "Phablet"],
             "adjs": ["5G", "AMOLED", "120Hz", "Triple-Camera", "Fast-Charging"],
             "brands": ["VeloPhone", "PrismMobile", "StarDevice", "ZephyrTech", "NexGen"]},
            {"name": "Audio & Headphones", "slug": "audio-headphones", "price": (19.99, 499.99),
             "nouns": ["Wireless Headphones", "Earbuds", "Speaker", "Soundbar", "DAC Amp", "In-Ear Monitor"],
             "adjs": ["Noise-Canceling", "Hi-Res", "Bluetooth 5.3", "Studio-Grade", "Waterproof"],
             "brands": ["SoundWave", "PureAudio", "BassLab", "ClearTone", "EchoMax"]},
            {"name": "Cameras & Photography", "slug": "cameras-photography", "price": (89.99, 2999.99),
             "nouns": ["Mirrorless Camera", "DSLR", "Action Camera", "Drone", "Camera Lens", "Tripod"],
             "adjs": ["4K Video", "Full-Frame", "Waterproof", "Stabilized", "24MP"],
             "brands": ["LensX", "OpticsPro", "SnapMaster", "VisionCraft", "FocusLab"]},
            {"name": "Smart Home & IoT", "slug": "smart-home", "price": (14.99, 349.99),
             "nouns": ["Smart Speaker", "Smart Bulb", "Security Camera", "Smart Plug", "Robot Vacuum", "Smart Lock"],
             "adjs": ["Voice-Controlled", "Wi-Fi 6", "Energy-Saving", "App-Connected", "AI-Powered"],
             "brands": ["SmartNest", "HomeIQ", "ConnectX", "AutoHome", "IntelliHome"]},
        ]
    },
    # ── Clothing & Fashion ────────────────────────────────────────────────────
    {
        "root": "Clothing & Fashion", "root_slug": "clothing-fashion",
        "seller_email": "seller02@fashionforward.com",
        "leaves": [
            {"name": "Men's Clothing", "slug": "mens-clothing", "price": (19.99, 199.99),
             "nouns": ["T-Shirt", "Dress Shirt", "Chinos", "Hoodie", "Blazer", "Polo Shirt", "Joggers"],
             "adjs": ["Slim Fit", "Stretch", "Merino Wool", "Breathable", "Wrinkle-Free"],
             "brands": ["ModernMen", "UrbanEdge", "ClassicWear", "DapperCo", "StyleX"]},
            {"name": "Women's Clothing", "slug": "womens-clothing", "price": (19.99, 249.99),
             "nouns": ["Dress", "Blouse", "Cardigan", "Skirt", "Jumpsuit", "Blazer", "Leggings"],
             "adjs": ["Floral", "Midi-Length", "Boho", "Structured", "Flowy"],
             "brands": ["ChicStyle", "FemmeX", "BlossomWear", "VogueX", "ElegantCo"]},
            {"name": "Footwear", "slug": "footwear", "price": (29.99, 299.99),
             "nouns": ["Sneakers", "Running Shoes", "Boots", "Loafers", "Sandals", "Dress Shoes", "Slip-Ons"],
             "adjs": ["Cushioned", "Memory Foam", "Waterproof", "Lightweight", "Anti-Slip"],
             "brands": ["StepUp", "KickX", "FootFlex", "SoleMate", "TreadPro"]},
            {"name": "Bags & Accessories", "slug": "bags-accessories", "price": (14.99, 199.99),
             "nouns": ["Backpack", "Tote Bag", "Crossbody Bag", "Wallet", "Belt", "Scarf", "Sunglasses"],
             "adjs": ["Vegan Leather", "Canvas", "RFID-Blocking", "Minimalist", "Spacious"],
             "brands": ["BagCraft", "AccesoX", "CarryStyle", "LuxBag", "PocketPro"]},
            {"name": "Watches & Jewelry", "slug": "watches-jewelry", "price": (19.99, 499.99),
             "nouns": ["Watch", "Bracelet", "Necklace", "Ring", "Earrings", "Smartwatch", "Pendant"],
             "adjs": ["Stainless Steel", "Rose Gold", "Minimalist", "Sapphire Crystal", "Engraved"],
             "brands": ["TimeCraft", "GemX", "WristPro", "LuxJewel", "ShimmerCo"]},
        ]
    },
    # ── Home & Garden ─────────────────────────────────────────────────────────
    {
        "root": "Home & Garden", "root_slug": "home-garden",
        "seller_email": "seller03@homeessentials.com",
        "leaves": [
            {"name": "Kitchen & Dining", "slug": "kitchen-dining", "price": (9.99, 299.99),
             "nouns": ["Air Fryer", "Coffee Maker", "Knife Set", "Cutting Board", "Mixing Bowl Set", "Instant Pot", "Blender"],
             "adjs": ["Non-Stick", "Stainless Steel", "Digital", "Dishwasher-Safe", "BPA-Free"],
             "brands": ["CookPro", "KitchenX", "ChefMate", "CuisineLab", "HomeCook"]},
            {"name": "Furniture & Living", "slug": "furniture-living", "price": (49.99, 999.99),
             "nouns": ["Sofa", "Coffee Table", "Bookshelf", "TV Stand", "Accent Chair", "Desk", "Nightstand"],
             "adjs": ["Mid-Century", "Scandinavian", "Space-Saving", "Solid Wood", "Upholstered"],
             "brands": ["FurnX", "HomeStyle", "LivingCo", "WoodCraft", "ModernHome"]},
            {"name": "Bedding & Bath", "slug": "bedding-bath", "price": (14.99, 199.99),
             "nouns": ["Duvet Cover", "Pillow", "Bed Sheet Set", "Towel Set", "Mattress Topper", "Blanket", "Bath Mat"],
             "adjs": ["100% Cotton", "Bamboo", "Cooling", "Weighted", "Hotel-Quality"],
             "brands": ["SleepWell", "BedLux", "ComfortX", "DreamNest", "PureHome"]},
            {"name": "Garden & Outdoor", "slug": "garden-outdoor", "price": (9.99, 299.99),
             "nouns": ["Garden Hose", "Planter Pot", "Garden Tools Set", "Lawn Mower", "Bird Feeder", "Outdoor Lights", "Hammock"],
             "adjs": ["Weather-Resistant", "Stainless Steel", "Solar-Powered", "Collapsible", "Heavy-Duty"],
             "brands": ["GardenX", "GreenThumb", "OutdoorPro", "BloomCo", "NatureCraft"]},
        ]
    },
    # ── Books & Media ─────────────────────────────────────────────────────────
    {
        "root": "Books & Media", "root_slug": "books-media",
        "seller_email": "seller04@bookverse.com",
        "leaves": [
            {"name": "Technology & Programming", "slug": "tech-books", "price": (9.99, 69.99),
             "nouns": ["Python Book", "JavaScript Guide", "Machine Learning Textbook", "System Design Book", "DevOps Handbook", "Cloud Computing Guide"],
             "adjs": ["Beginner-Friendly", "Comprehensive", "Updated 2024", "Hands-On", "Best-Selling"],
             "brands": ["CodePress", "TechBooks", "DevLibrary", "LearnPub", "ProgrammerPress"]},
            {"name": "Fiction & Literature", "slug": "fiction-literature", "price": (7.99, 29.99),
             "nouns": ["Novel", "Thriller", "Mystery", "Romance", "Science Fiction", "Fantasy Novel", "Short Stories"],
             "adjs": ["Award-Winning", "Bestselling", "Page-Turner", "Gripping", "Critically Acclaimed"],
             "brands": ["StoryPress", "FictionHouse", "NarrativeX", "LitWorld", "ReadMore"]},
            {"name": "Science & Education", "slug": "science-education", "price": (14.99, 89.99),
             "nouns": ["Biology Textbook", "Physics Guide", "Chemistry Manual", "Math Workbook", "Astronomy Book", "Neuroscience Book"],
             "adjs": ["Illustrated", "Revised Edition", "College-Level", "Research-Based", "Peer-Reviewed"],
             "brands": ["SciencePress", "EduBooks", "AcademicX", "LearnSci", "StudyPro"]},
            {"name": "History & Biography", "slug": "history-biography", "price": (9.99, 39.99),
             "nouns": ["Biography", "Autobiography", "History Book", "Memoir", "World War Book", "Ancient History"],
             "adjs": ["Definitive", "Illustrated", "Pulitzer Prize-Winning", "Landmark", "Revised"],
             "brands": ["HistoryX", "BioPub", "ChronicleBooks", "PastPress", "LegacyRead"]},
        ]
    },
    # ── Sports & Outdoors ─────────────────────────────────────────────────────
    {
        "root": "Sports & Outdoors", "root_slug": "sports-outdoors",
        "seller_email": "seller05@sportspeak.com",
        "leaves": [
            {"name": "Fitness Equipment", "slug": "fitness-equipment", "price": (19.99, 799.99),
             "nouns": ["Dumbbell Set", "Resistance Bands", "Yoga Mat", "Pull-Up Bar", "Treadmill", "Kettlebell", "Foam Roller"],
             "adjs": ["Anti-Slip", "Adjustable", "Commercial-Grade", "Compact", "Heavy-Duty"],
             "brands": ["FitPro", "IronX", "GymCore", "StrengthCo", "ActiveX"]},
            {"name": "Outdoor & Camping", "slug": "outdoor-camping", "price": (14.99, 499.99),
             "nouns": ["Tent", "Sleeping Bag", "Hiking Backpack", "Camping Stove", "Headlamp", "Trekking Poles", "Water Filter"],
             "adjs": ["Lightweight", "Waterproof", "4-Season", "Ultralight", "Wind-Resistant"],
             "brands": ["TrailBlaze", "CampX", "OutdoorPro", "WildGear", "ExploreX"]},
            {"name": "Cycling & Accessories", "slug": "cycling", "price": (14.99, 399.99),
             "nouns": ["Bike Helmet", "Cycling Gloves", "Bike Lock", "Cycling Jersey", "Bike Computer", "Saddle Bag", "Water Bottle Cage"],
             "adjs": ["Aerodynamic", "Reflective", "Breathable", "Anti-Theft", "Lightweight"],
             "brands": ["CycleX", "VeloGear", "RidePro", "SpinCo", "BikeMate"]},
            {"name": "Water Sports", "slug": "water-sports", "price": (19.99, 599.99),
             "nouns": ["Swim Goggles", "Wetsuit", "Paddle Board", "Snorkel Set", "Life Jacket", "Kayak Paddle", "Dry Bag"],
             "adjs": ["UV-Protection", "Anti-Fog", "Buoyant", "Flexible", "Quick-Dry"],
             "brands": ["AquaX", "WaveRider", "SplashPro", "OceanGear", "HydroSport"]},
        ]
    },
    # ── Beauty & Personal Care ────────────────────────────────────────────────
    {
        "root": "Beauty & Personal Care", "root_slug": "beauty-care",
        "seller_email": "seller06@beautybox.com",
        "leaves": [
            {"name": "Skincare", "slug": "skincare", "price": (8.99, 129.99),
             "nouns": ["Serum", "Moisturizer", "Face Wash", "Toner", "Eye Cream", "SPF Sunscreen", "Face Mask"],
             "adjs": ["Vitamin C", "Retinol", "Hyaluronic Acid", "Niacinamide", "Anti-Aging"],
             "brands": ["GlowLab", "PureSkin", "DermaFix", "LumiGlow", "ClearPore"]},
            {"name": "Haircare", "slug": "haircare", "price": (7.99, 79.99),
             "nouns": ["Shampoo", "Conditioner", "Hair Mask", "Hair Oil", "Dry Shampoo", "Leave-In Spray"],
             "adjs": ["Argan Oil", "Keratin", "Color-Safe", "Volumizing", "Repair Formula"],
             "brands": ["SilkMane", "HydraHair", "ReviveX", "GlossyLocks", "HairLab"]},
            {"name": "Makeup & Cosmetics", "slug": "makeup-cosmetics", "price": (6.99, 89.99),
             "nouns": ["Foundation", "Mascara", "Lipstick", "Eyeshadow Palette", "Blush", "Highlighter", "Concealer"],
             "adjs": ["Long-Lasting", "Matte", "Full-Coverage", "Buildable", "Vegan"],
             "brands": ["GlamourX", "FaceForward", "ColorStudio", "BeautyPop", "PigmentPro"]},
            {"name": "Fragrances", "slug": "fragrances", "price": (19.99, 249.99),
             "nouns": ["Eau de Parfum", "Cologne", "Body Mist", "Perfume Oil", "Gift Set"],
             "adjs": ["Floral", "Woody", "Fresh", "Oriental", "Citrus"],
             "brands": ["ScentLux", "AromaElite", "FragranceHouse", "NotesByCo", "EssenceX"]},
            {"name": "Men's Grooming", "slug": "mens-grooming", "price": (9.99, 69.99),
             "nouns": ["Beard Oil", "Shaving Cream", "Aftershave", "Face Scrub", "Beard Balm", "Electric Razor"],
             "adjs": ["Sandalwood", "Charcoal", "Cooling", "Hydrating", "Sensitive-Skin"],
             "brands": ["GroomCo", "ManEdge", "BarbershopX", "BladeKing", "FaceForce"]},
        ]
    },
    # ── Baby & Kids ───────────────────────────────────────────────────────────
    {
        "root": "Baby & Kids", "root_slug": "baby-kids",
        "seller_email": "seller07@babybliss.com",
        "leaves": [
            {"name": "Baby Gear & Safety", "slug": "baby-gear", "price": (19.99, 499.99),
             "nouns": ["Stroller", "Baby Monitor", "Car Seat", "Baby Carrier", "Bouncer Seat", "Baby Gate"],
             "adjs": ["Lightweight", "Foldable", "Smart", "Safety-Certified", "All-Terrain"],
             "brands": ["TinyStep", "BabyGuard", "SafeRide", "MiniMove", "PureStart"]},
            {"name": "Toys & Games", "slug": "toys-games", "price": (9.99, 149.99),
             "nouns": ["Building Blocks", "Puzzle", "Action Figure", "Board Game", "Remote Car", "Doll", "Plush Toy"],
             "adjs": ["Interactive", "Educational", "Non-Toxic", "Colorful", "Durable"],
             "brands": ["PlayWorld", "KidsBright", "FunZone", "ToyLab", "CreatiKids"]},
            {"name": "Educational Toys", "slug": "educational-toys", "price": (14.99, 99.99),
             "nouns": ["Math Kit", "Science Set", "Coding Robot", "Flash Cards", "Globe", "Telescope", "Microscope"],
             "adjs": ["STEM", "Montessori", "Award-Winning", "Screen-Free", "Hands-On"],
             "brands": ["BrainBoost", "LearnPlay", "SmartKid", "EduFun", "MindBuilder"]},
        ]
    },
    # ── Pets ──────────────────────────────────────────────────────────────────
    {
        "root": "Pets", "root_slug": "pets",
        "seller_email": "seller08@petparadise.com",
        "leaves": [
            {"name": "Dog Supplies", "slug": "dog-supplies", "price": (4.99, 199.99),
             "nouns": ["Dog Food", "Leash", "Harness", "Dog Bed", "Chew Toy", "Dog Crate", "Grooming Brush"],
             "adjs": ["Grain-Free", "Adjustable", "Orthopedic", "Indestructible", "Reflective"],
             "brands": ["PawsFirst", "DogWorld", "TailWag", "WoofCo", "PetPro"]},
            {"name": "Cat Supplies", "slug": "cat-supplies", "price": (4.99, 149.99),
             "nouns": ["Cat Food", "Cat Litter", "Scratching Post", "Cat Tree", "Litter Box", "Cat Toy", "Cat Carrier"],
             "adjs": ["Self-Cleaning", "Odor-Control", "Sisal", "Multi-Level", "Interactive"],
             "brands": ["PurrFect", "MeowCo", "CatLux", "WhiskerX", "FelinePro"]},
            {"name": "Bird Supplies", "slug": "bird-supplies", "price": (9.99, 129.99),
             "nouns": ["Bird Cage", "Bird Food", "Perch", "Nest Box", "Swing Toy", "Bird Bath", "Mineral Block"],
             "adjs": ["Stainless Steel", "Spacious", "Easy-Clean", "Natural Wood", "Colorful"],
             "brands": ["WingPro", "BirdHome", "FeatherX", "AvianCare", "NestMate"]},
            {"name": "Fish & Aquarium", "slug": "fish-aquarium", "price": (7.99, 299.99),
             "nouns": ["Aquarium Tank", "Filter", "Heater", "Fish Food", "Gravel", "LED Light", "Air Pump"],
             "adjs": ["20-Gallon", "Freshwater", "Saltwater", "Submersible", "Quiet-Flow"],
             "brands": ["AquaPro", "FishTank", "WaterWorld", "ClearStream", "AquaLife"]},
        ]
    },
    # ── Food & Grocery ────────────────────────────────────────────────────────
    {
        "root": "Food & Grocery", "root_slug": "food-grocery",
        "seller_email": "seller09@foodcraft.com",
        "leaves": [
            {"name": "Snacks & Chips", "slug": "snacks", "price": (1.99, 24.99),
             "nouns": ["Chips", "Popcorn", "Nuts Mix", "Crackers", "Granola Bar", "Trail Mix", "Pretzels"],
             "adjs": ["Sea Salt", "Spicy", "Organic", "Gluten-Free", "Low-Calorie"],
             "brands": ["CrunchCo", "SnackBurst", "NatureBites", "CrispyPop", "GoodMunch"]},
            {"name": "Coffee & Tea", "slug": "coffee-tea", "price": (7.99, 59.99),
             "nouns": ["Ground Coffee", "Whole Bean Coffee", "Green Tea", "Herbal Tea", "Cold Brew", "Espresso Pods"],
             "adjs": ["Single-Origin", "Dark Roast", "Organic", "Fairtrade", "Medium Roast"],
             "brands": ["BrewMaster", "LeafOrigin", "CoffeeCraft", "TeaHaven", "PureRoast"]},
            {"name": "Organic & Natural Foods", "slug": "organic-foods", "price": (4.99, 49.99),
             "nouns": ["Quinoa", "Chia Seeds", "Coconut Oil", "Almond Butter", "Oat Flour", "Raw Honey", "Flax Seeds"],
             "adjs": ["USDA Organic", "Non-GMO", "Raw", "Cold-Pressed", "100% Natural"],
             "brands": ["OrganicRoots", "EarthFirst", "PureHarvest", "GreenField", "NaturePure"]},
            {"name": "Beverages & Drinks", "slug": "beverages", "price": (1.49, 39.99),
             "nouns": ["Sparkling Water", "Protein Shake", "Energy Drink", "Juice", "Kombucha", "Electrolyte Drink"],
             "adjs": ["Sugar-Free", "Zero-Calorie", "Vitamin-Enriched", "Natural", "Carbonated"],
             "brands": ["HydroBoost", "VitaSip", "RefreshCo", "DrinkWell", "BubbleX"]},
        ]
    },
    # ── Musical Instruments ───────────────────────────────────────────────────
    {
        "root": "Musical Instruments", "root_slug": "musical-instruments",
        "seller_email": "seller10@musiczone.com",
        "leaves": [
            {"name": "Guitars & Bass", "slug": "guitars-bass", "price": (79.99, 1499.99),
             "nouns": ["Acoustic Guitar", "Electric Guitar", "Bass Guitar", "Classical Guitar", "Travel Guitar", "Guitar Bundle"],
             "adjs": ["Solid-Top", "Mahogany", "Rosewood Fingerboard", "Sunburst Finish", "Beginner-Friendly"],
             "brands": ["StringMaster", "ChordCraft", "MelodyX", "FretBoard", "SoundWave"]},
            {"name": "Keyboards & Pianos", "slug": "keyboards-pianos", "price": (99.99, 1999.99),
             "nouns": ["Digital Piano", "MIDI Keyboard", "Synthesizer", "Stage Piano", "Arranger Keyboard", "Mini Keyboard"],
             "adjs": ["88-Key", "Weighted Keys", "Semi-Weighted", "Bluetooth", "Portable"],
             "brands": ["KeyMaster", "PianoX", "SynthPro", "DigitalKeys", "IvoryTouch"]},
            {"name": "Drums & Percussion", "slug": "drums-percussion", "price": (49.99, 999.99),
             "nouns": ["Electronic Drum Kit", "Snare Drum", "Djembe", "Cajon", "Drum Pad", "Cymbal Set", "Drum Sticks"],
             "adjs": ["Mesh-Head", "Professional", "Practice-Ready", "Acoustic", "8-Piece"],
             "brands": ["BeatCraft", "RhythmX", "DrumMaster", "PercussionPro", "StickHit"]},
            {"name": "Studio Equipment", "slug": "studio-equipment", "price": (29.99, 799.99),
             "nouns": ["Audio Interface", "Studio Monitor", "Condenser Microphone", "Studio Headphones", "MIDI Controller", "Pop Filter"],
             "adjs": ["XLR", "USB", "Studio-Grade", "Dynamic", "Professional"],
             "brands": ["StudioX", "RecordPro", "AudioCraft", "SoundBooth", "MixMaster"]},
        ]
    },
]


def _leaf_categories():
    return [
        {**leaf, "_root": group["root"], "_root_slug": group["root_slug"], "_seller_email": group["seller_email"]}
        for group in TAXONOMY
        for leaf in group["leaves"]
    ]


LEAF_CATEGORIES = _leaf_categories()


# ── Image helpers ──────────────────────────────────────────────────────────────

def _name_slug(name: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")[:40]


# ── Seeding functions ──────────────────────────────────────────────────────────

def cleanup_data(users_conn, prod_conn, carts_conn, orders_conn, payments_conn, dry_run: bool):
    """Wipe all transactional and seed data, preserving the 3 sample users."""
    if dry_run:
        print("  [dry-run] would truncate products, categories, carts, orders, payments DBs")
        print("  [dry-run] would delete all sellers except protected sample users")
        return

    # ── Products DB (leaf tables first; categories.id → products.category_id is SET NULL) ──
    for stmt in [
        "TRUNCATE stock_movements RESTART IDENTITY CASCADE",
        "TRUNCATE product_reviews RESTART IDENTITY CASCADE",
        "TRUNCATE product_images RESTART IDENTITY CASCADE",
        "TRUNCATE products RESTART IDENTITY CASCADE",
        "TRUNCATE categories RESTART IDENTITY CASCADE",
    ]:
        prod_conn.execute(stmt)
    prod_conn.commit()

    # ── Users DB (ON DELETE CASCADE handles profiles, addresses, tokens) ──
    placeholders = ", ".join(f"'{uid}'" for uid in PROTECTED_USER_IDS)
    users_conn.execute(
        f"DELETE FROM users WHERE role = 'seller' AND id NOT IN ({placeholders})"
    )
    users_conn.commit()

    # ── Carts DB ──
    carts_conn.execute("TRUNCATE cart_items, carts RESTART IDENTITY CASCADE")
    carts_conn.commit()

    # ── Orders DB (notifications has no CASCADE from orders — must go first) ──
    for stmt in [
        "TRUNCATE orders_outbox RESTART IDENTITY CASCADE",
        "TRUNCATE notifications RESTART IDENTITY CASCADE",
        "TRUNCATE order_status_history RESTART IDENTITY CASCADE",
        "TRUNCATE order_items RESTART IDENTITY CASCADE",
        "TRUNCATE orders RESTART IDENTITY CASCADE",
    ]:
        orders_conn.execute(stmt)
    orders_conn.commit()

    # ── Payments DB ──
    payments_conn.execute("TRUNCATE payment_history, payments RESTART IDENTITY CASCADE")
    payments_conn.commit()


def seed_sellers(users_conn, dry_run: bool) -> dict:
    """Insert 10 sellers. Returns {email: uuid_str}."""
    pw_hash = bcrypt.hashpw(SELLER_PASSWORD.encode(), bcrypt.gensalt(rounds=10)).decode()
    for s in SELLERS:
        if not dry_run:
            users_conn.execute(
                """
                INSERT INTO users (id, email, password_hash, role, is_verified, verified_at)
                VALUES (%s, %s, %s, 'seller', TRUE, NOW())
                ON CONFLICT (email) DO NOTHING
                """,
                (s["id"], s["email"], pw_hash),
            )
            users_conn.execute(
                """
                INSERT INTO user_profiles (user_id, first_name, last_name)
                VALUES (%s, %s, %s)
                ON CONFLICT (user_id) DO NOTHING
                """,
                (s["id"], s["first_name"], s["last_name"]),
            )
    if not dry_run:
        users_conn.commit()
    return {s["email"]: s["id"] for s in SELLERS}


def seed_categories(prod_conn, dry_run: bool) -> dict:
    """Insert root + leaf categories. Returns {leaf_slug: category_id}."""
    sort_order = 0

    for group in TAXONOMY:
        sort_order += 1
        if not dry_run:
            prod_conn.execute(
                """
                INSERT INTO categories (name, slug, parent_id, sort_order)
                VALUES (%s, %s, NULL, %s)
                ON CONFLICT (slug) DO NOTHING
                """,
                (group["root"], group["root_slug"], sort_order),
            )

    if not dry_run:
        prod_conn.commit()

    # Fetch all root IDs
    root_id_by_slug = {}
    if not dry_run:
        rows = prod_conn.execute(
            "SELECT slug, id FROM categories WHERE parent_id IS NULL"
        ).fetchall()
        root_id_by_slug = {r[0]: r[1] for r in rows}

    leaf_sort = 0
    for group in TAXONOMY:
        root_id = root_id_by_slug.get(group["root_slug"])
        for leaf in group["leaves"]:
            leaf_sort += 1
            if not dry_run:
                prod_conn.execute(
                    """
                    INSERT INTO categories (name, slug, parent_id, sort_order)
                    VALUES (%s, %s, %s, %s)
                    ON CONFLICT (slug) DO NOTHING
                    """,
                    (leaf["name"], leaf["slug"], root_id, leaf_sort),
                )

    if not dry_run:
        prod_conn.commit()

    cat_id_map = {}
    if not dry_run:
        rows = prod_conn.execute(
            "SELECT slug, id FROM categories WHERE parent_id IS NOT NULL"
        ).fetchall()
        cat_id_map = {slug: cid for slug, cid in rows}

    return cat_id_map


def _make_name(leaf: dict) -> str:
    brand = random.choice(leaf["brands"])
    adj   = random.choice(leaf["adjs"])
    noun  = random.choice(leaf["nouns"])
    suffix = random.choice(["", " Pro", " Plus", " Lite", " Max", " X", " 2.0", " Premium", ""])
    return f"{brand} {adj} {noun}{suffix}"


def _make_description(leaf: dict) -> str:
    noun  = random.choice(leaf["nouns"])
    adj1  = random.choice(leaf["adjs"])
    adj2  = random.choice(leaf["adjs"])
    cat   = leaf["name"]
    options = [
        f"{adj1} {noun} for everyday use. Perfect for {cat} enthusiasts. {adj2} design with premium materials.",
        f"High-quality {noun} featuring {adj1} technology. Ideal for {cat}. Includes {adj2} finish.",
        f"Professional-grade {noun}. {adj1} performance meets {adj2} durability. Trusted by {cat} professionals.",
        f"Compact {adj1} {noun} with {adj2} build quality. Best in class for {cat} needs.",
        f"{adj1} {noun} — redefining {cat}. {adj2} construction, long-lasting performance.",
    ]
    return random.choice(options)


def _make_status(i: int) -> str:
    if i % 20 == 0:
        return "DELETED"
    if i % 10 == 0:
        return "INACTIVE"
    return "ACTIVE"


def seed_products(prod_conn, cat_id_map: dict, seller_id_by_leaf_slug: dict, target: int, dry_run: bool) -> int:
    """Stream products via COPY. Returns the min inserted product id."""
    if dry_run:
        print(f"  [dry-run] would insert {target:,} products")
        return -1

    available_leaves = [lf for lf in LEAF_CATEGORIES if lf["slug"] in cat_id_map]
    if not available_leaves:
        raise RuntimeError("No leaf categories found — run seed_categories first.")

    leaf_cycle = itertools.cycle(available_leaves)

    row = prod_conn.execute("SELECT COALESCE(MAX(id), 0) FROM products").fetchone()
    id_before = row[0]

    with prod_conn.cursor().copy(
        "COPY products (name, description, price, category_id, seller_id, status, stock_quantity, stock_reserved, version) FROM STDIN"
    ) as copy:
        for i in range(1, target + 1):
            leaf      = next(leaf_cycle)
            cat_id    = cat_id_map[leaf["slug"]]
            seller_id = seller_id_by_leaf_slug[leaf["slug"]]
            status    = _make_status(i)
            stock_qty = random.randint(10, 500)
            stock_res = random.randint(0, min(10, stock_qty)) if status == "ACTIVE" else 0
            price     = round(random.uniform(*leaf["price"]), 2)
            copy.write_row((
                _make_name(leaf),
                _make_description(leaf),
                str(price),
                cat_id,
                seller_id,
                status,
                stock_qty,
                stock_res,
                0,
            ))

    prod_conn.commit()

    row = prod_conn.execute(
        "SELECT COALESCE(MIN(id), 0) FROM products WHERE id > %s", (id_before,)
    ).fetchone()
    return row[0]


def seed_images(prod_conn, min_product_id: int, dry_run: bool) -> int:
    """Give every product exactly 2 picsum images with name-slug seeds."""
    if dry_run or min_product_id <= 0:
        return 0

    ids = prod_conn.execute(
        "SELECT id, name FROM products WHERE id >= %s ORDER BY id", (min_product_id,)
    ).fetchall()

    with prod_conn.cursor().copy(
        "COPY product_images (product_id, url, alt_text, sort_order) FROM STDIN"
    ) as copy:
        for pid, name in ids:
            slug = _name_slug(name)
            copy.write_row((pid, f"https://picsum.photos/seed/{slug}-primary/600/400",
                            f"{name[:80]} — main view", 0))
            copy.write_row((pid, f"https://picsum.photos/seed/{slug}-alt/600/400",
                            f"{name[:80]} — alternate view", 1))

    prod_conn.commit()
    return len(ids)


# ── Main ───────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Clean and reseed ecommerce data.")
    parser.add_argument("--target",       type=int,          default=10_000, help="Products to seed (default: 10000)")
    parser.add_argument("--skip-clean",   action="store_true", help="Skip cleanup step (additive mode)")
    parser.add_argument("--skip-sellers", action="store_true", help="Skip seller user seeding")
    parser.add_argument("--skip-images",  action="store_true", help="Skip product image seeding")
    parser.add_argument("--dry-run",      action="store_true", help="Print only — no DB writes")
    args = parser.parse_args()

    random.seed(42)
    t0 = time.time()

    print(f"{'[DRY RUN] ' if args.dry_run else ''}Connecting to all 5 databases...")
    users_conn    = psycopg.connect(USERS_DSN)
    prod_conn     = psycopg.connect(PRODUCTS_DSN)
    carts_conn    = psycopg.connect(CARTS_DSN)
    orders_conn   = psycopg.connect(ORDERS_DSN)
    payments_conn = psycopg.connect(PAYMENTS_DSN)

    # 1. Cleanup
    if not args.skip_clean:
        print("Cleaning existing data (products, categories, carts, orders, payments, sellers)...", end=" ", flush=True)
        cleanup_data(users_conn, prod_conn, carts_conn, orders_conn, payments_conn, args.dry_run)
        print("done")
    else:
        print("Skipping cleanup (--skip-clean)")

    # 2. Sellers
    if not args.skip_sellers:
        print(f"Seeding {len(SELLERS)} sellers...", end=" ", flush=True)
        seller_email_to_id = seed_sellers(users_conn, args.dry_run)
        print(f"done")
    else:
        seller_email_to_id = {s["email"]: s["id"] for s in SELLERS}
        print(f"Skipping sellers — using pre-defined UUIDs")

    # 3. Categories
    print("Seeding categories...", end=" ", flush=True)
    cat_id_map = seed_categories(prod_conn, args.dry_run)
    n_roots  = len(TAXONOMY)
    n_leaves = sum(len(g["leaves"]) for g in TAXONOMY)
    print(f"done ({n_roots} root, {n_leaves} leaf categories)")

    # 4. Build seller-by-leaf-slug mapping
    seller_id_by_leaf_slug = {
        leaf["slug"]: seller_email_to_id.get(group["seller_email"], group["seller_email"])
        for group in TAXONOMY
        for leaf in group["leaves"]
    }

    # 5. Products
    print(f"Seeding {args.target:,} products via COPY...", end=" ", flush=True)
    t_prod = time.time()
    min_id = seed_products(prod_conn, cat_id_map, seller_id_by_leaf_slug, args.target, args.dry_run)
    print(f"done in {time.time() - t_prod:.1f}s")

    # 6. Images
    if not args.skip_images:
        print("Seeding product images via COPY (2 per product)...", end=" ", flush=True)
        t_img = time.time()
        n_products = seed_images(prod_conn, min_id, args.dry_run)
        if n_products:
            print(f"done in {time.time() - t_img:.1f}s ({n_products * 2:,} images for {n_products:,} products)")
        else:
            print("skipped")

    users_conn.close()
    prod_conn.close()
    carts_conn.close()
    orders_conn.close()
    payments_conn.close()

    print(f"\nTotal elapsed: {time.time() - t0:.1f}s")

    if not args.dry_run and not args.skip_sellers:
        print(f"\nSeller accounts (password: {SELLER_PASSWORD}):")
        for s in SELLERS:
            print(f"  {s['email']:<38} → {s['id']}  [{s['store']}]")

    print("\nNext step — generate embeddings:")
    print("  docker exec -it ecommerce-ai-service python scripts/embed_products.py")
    print("\n  (or expose port 9000 in docker-compose.yml and run:)")
    print("  python3 script/embed_products.py")


if __name__ == "__main__":
    main()
