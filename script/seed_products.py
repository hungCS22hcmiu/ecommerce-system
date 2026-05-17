#!/usr/bin/env python3
"""
Seed 15 sellers + ~100,000 diverse products into the ecommerce system.

Usage:
  python3 script/seed_products.py
  python3 script/seed_products.py --target 10000
  python3 script/seed_products.py --skip-sellers --skip-images
  python3 script/seed_products.py --dry-run

Prerequisites:
  pip install "psycopg[binary]" bcrypt

Env vars (defaults match docker-compose localhost exposure):
  USERS_DB_HOST, USERS_DB_PORT, USERS_DB_NAME, USERS_DB_USER, USERS_DB_PASSWORD
  PRODUCTS_DB_HOST, PRODUCTS_DB_PORT, PRODUCTS_DB_NAME, PRODUCTS_DB_USER, PRODUCTS_DB_PASSWORD
"""
import argparse
import itertools
import os
import random
import time
from decimal import Decimal

import bcrypt
import psycopg

# ── DB connections ─────────────────────────────────────────────────────────────

USERS_DSN = (
    f"host={os.getenv('USERS_DB_HOST', 'localhost')} "
    f"port={os.getenv('USERS_DB_PORT', '5432')} "
    f"dbname={os.getenv('USERS_DB_NAME', 'ecommerce_users')} "
    f"user={os.getenv('USERS_DB_USER', 'postgres')} "
    f"password={os.getenv('USERS_DB_PASSWORD', 'postgres')}"
)
PRODUCTS_DSN = (
    f"host={os.getenv('PRODUCTS_DB_HOST', 'localhost')} "
    f"port={os.getenv('PRODUCTS_DB_PORT', '5432')} "
    f"dbname={os.getenv('PRODUCTS_DB_NAME', 'ecommerce_products')} "
    f"user={os.getenv('PRODUCTS_DB_USER', 'postgres')} "
    f"password={os.getenv('PRODUCTS_DB_PASSWORD', 'postgres')}"
)

SELLER_PASSWORD = "Password123!"

# ── Sellers (15 total — first 3 match V2 seed placeholders) ───────────────────

SELLERS = [
    {"id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller01@techstore.com",   "first_name": "Alex",    "last_name": "Chen"},
    {"id": "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller02@fashionhub.com",  "first_name": "Maria",   "last_name": "Santos"},
    {"id": "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller03@homegoods.com",   "first_name": "James",   "last_name": "Wilson"},
    {"id": "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller04@sportzone.com",   "first_name": "Priya",   "last_name": "Patel"},
    {"id": "e0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller05@beautyco.com",    "first_name": "Sophie",  "last_name": "Martin"},
    {"id": "f0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller06@bookworld.com",   "first_name": "David",   "last_name": "Kim"},
    {"id": "a1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller07@autoparts.com",   "first_name": "Carlos",  "last_name": "Gomez"},
    {"id": "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller08@petshop.com",     "first_name": "Emma",    "last_name": "Johnson"},
    {"id": "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller09@gamevault.com",   "first_name": "Liam",    "last_name": "Brown"},
    {"id": "d1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller10@jewelplus.com",   "first_name": "Aisha",   "last_name": "Okafor"},
    {"id": "e1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller11@musicmart.com",   "first_name": "Noah",    "last_name": "Taylor"},
    {"id": "f1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller12@officesupply.com","first_name": "Yuki",    "last_name": "Tanaka"},
    {"id": "a2eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller13@babyworld.com",   "first_name": "Fatima",  "last_name": "Hassan"},
    {"id": "b2eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller14@travelpro.com",   "first_name": "Oliver",  "last_name": "Smith"},
    {"id": "c2eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "email": "seller15@healthplus.com",  "first_name": "Isabella","last_name": "Rossi"},
]

# ── Category taxonomy ──────────────────────────────────────────────────────────
# Each leaf has: name, slug, price_range (min, max), nouns, adjectives, brands

TAXONOMY = [
    # ── Automotive ────────────────────────────────────────────────────────────
    {
        "root": "Automotive", "root_slug": "automotive",
        "leaves": [
            {"name": "Car Care", "slug": "car-care", "price": (5.99, 89.99),
             "nouns": ["Wax", "Polish", "Shampoo", "Cleaner", "Detailer", "Coating"],
             "adjs": ["Premium", "Ultra-Shine", "Heavy-Duty", "Ceramic", "Waterless"],
             "brands": ["AutoShine", "MeguiarsPro", "ChemX", "DetailKing", "ClearCoat"]},
            {"name": "Car Electronics", "slug": "car-electronics", "price": (19.99, 349.99),
             "nouns": ["Dash Cam", "GPS Navigator", "Backup Camera", "Car Charger", "FM Transmitter"],
             "adjs": ["4K", "Wireless", "HD", "Dual-Channel", "Smart"],
             "brands": ["DriveView", "NavPro", "VisionX", "AutoConnect", "SmartDrive"]},
            {"name": "Interior Accessories", "slug": "car-interior", "price": (9.99, 129.99),
             "nouns": ["Seat Cover", "Floor Mat", "Steering Wheel Cover", "Car Organizer", "Sunshade"],
             "adjs": ["Universal", "Leather", "Custom-Fit", "Premium", "All-Season"],
             "brands": ["ComfortRide", "LuxAuto", "FitPerfect", "DriveComfort", "AutoStyle"]},
            {"name": "Exterior Accessories", "slug": "car-exterior", "price": (14.99, 199.99),
             "nouns": ["Car Cover", "Mud Flap", "Roof Rack", "Tow Hook", "Window Visor"],
             "adjs": ["Weatherproof", "Heavy-Duty", "Universal", "Aerodynamic", "UV-Resistant"],
             "brands": ["ShieldAuto", "ExterioPro", "WeatherGuard", "TopMount", "DriveX"]},
        ]
    },
    # ── Beauty & Personal Care ────────────────────────────────────────────────
    {
        "root": "Beauty & Personal Care", "root_slug": "beauty",
        "leaves": [
            {"name": "Skincare", "slug": "skincare", "price": (8.99, 129.99),
             "nouns": ["Serum", "Moisturizer", "Toner", "Face Wash", "Eye Cream", "SPF Sunscreen"],
             "adjs": ["Vitamin C", "Retinol", "Hyaluronic", "Niacinamide", "Anti-Aging"],
             "brands": ["GlowLab", "PureSkin", "DermaFix", "LumiGlow", "ClearPore"]},
            {"name": "Haircare", "slug": "haircare", "price": (7.99, 79.99),
             "nouns": ["Shampoo", "Conditioner", "Hair Mask", "Hair Oil", "Leave-In Spray", "Dry Shampoo"],
             "adjs": ["Argan Oil", "Keratin", "Color-Safe", "Volumizing", "Repair"],
             "brands": ["SilkMane", "HydraHair", "ReviveX", "GlossyLocks", "HairLab"]},
            {"name": "Makeup", "slug": "makeup", "price": (6.99, 89.99),
             "nouns": ["Foundation", "Mascara", "Lipstick", "Eyeshadow Palette", "Blush", "Highlighter"],
             "adjs": ["Long-Lasting", "Matte", "Dewy", "Full-Coverage", "Buildable"],
             "brands": ["GlamourX", "FaceForward", "ColorStudio", "BeautyPop", "PigmentPro"]},
            {"name": "Fragrances", "slug": "fragrances", "price": (19.99, 249.99),
             "nouns": ["Eau de Parfum", "Cologne", "Body Mist", "Perfume Oil", "Deodorant"],
             "adjs": ["Floral", "Woody", "Fresh", "Oriental", "Citrus"],
             "brands": ["ScentLux", "AromaElite", "FragranceHouse", "NotesByCo", "EssenceX"]},
            {"name": "Men's Grooming", "slug": "mens-grooming", "price": (9.99, 69.99),
             "nouns": ["Beard Oil", "Shaving Cream", "Aftershave", "Face Scrub", "Beard Balm"],
             "adjs": ["Sandalwood", "Charcoal", "Cooling", "Hydrating", "Sensitive-Skin"],
             "brands": ["GroomCo", "ManEdge", "BarbershopX", "BladeKing", "FaceForce"]},
        ]
    },
    # ── Baby & Kids ───────────────────────────────────────────────────────────
    {
        "root": "Baby & Kids", "root_slug": "baby-kids",
        "leaves": [
            {"name": "Baby Gear", "slug": "baby-gear", "price": (19.99, 499.99),
             "nouns": ["Stroller", "Baby Monitor", "Car Seat", "Baby Carrier", "Bouncer"],
             "adjs": ["Lightweight", "Foldable", "Smart", "Safety-Certified", "All-Terrain"],
             "brands": ["TinyStep", "BabyGuard", "SafeRide", "MiniMove", "PureStart"]},
            {"name": "Toys & Games", "slug": "kids-toys", "price": (9.99, 149.99),
             "nouns": ["Building Blocks", "Puzzle", "Action Figure", "Board Game", "Remote Car", "Doll"],
             "adjs": ["Interactive", "Educational", "Creative", "STEM", "Colorful"],
             "brands": ["PlayWorld", "KidsBright", "FunZone", "ToyLab", "CreatiKids"]},
            {"name": "Educational Toys", "slug": "educational-toys", "price": (14.99, 99.99),
             "nouns": ["Math Kit", "Science Set", "Coding Robot", "Flash Cards", "Globe", "Telescope"],
             "adjs": ["STEM", "Montessori", "Award-Winning", "Screen-Free", "Hands-On"],
             "brands": ["BrainBoost", "LearnPlay", "SmartKid", "EduFun", "MindBuilder"]},
            {"name": "Kids Clothing", "slug": "kids-clothing", "price": (9.99, 59.99),
             "nouns": ["T-Shirt", "Pajamas", "Hoodie", "Dress", "Jacket", "Shorts"],
             "adjs": ["100% Cotton", "Soft", "Machine-Washable", "Colorful", "Breathable"],
             "brands": ["TinyThreads", "KidsWear", "LittleStyle", "MiniFashion", "CozyKids"]},
        ]
    },
    # ── Food & Grocery ────────────────────────────────────────────────────────
    {
        "root": "Food & Grocery", "root_slug": "food-grocery",
        "leaves": [
            {"name": "Snacks & Chips", "slug": "snacks", "price": (1.99, 24.99),
             "nouns": ["Chips", "Popcorn", "Nuts Mix", "Crackers", "Granola Bar", "Trail Mix"],
             "adjs": ["Sea Salt", "Spicy", "Organic", "Gluten-Free", "Low-Calorie"],
             "brands": ["CrunchCo", "SnackBurst", "NatureBites", "CrispyPop", "GoodMunch"]},
            {"name": "Coffee & Tea", "slug": "coffee-tea", "price": (7.99, 59.99),
             "nouns": ["Ground Coffee", "Whole Bean", "Green Tea", "Herbal Tea", "Cold Brew", "Espresso"],
             "adjs": ["Single-Origin", "Dark Roast", "Organic", "Fairtrade", "Decaf"],
             "brands": ["BrewMaster", "LeafOrigin", "CoffeeCraft", "TeaHaven", "PureRoast"]},
            {"name": "Organic Foods", "slug": "organic-foods", "price": (4.99, 49.99),
             "nouns": ["Quinoa", "Chia Seeds", "Coconut Oil", "Almond Butter", "Oat Flour", "Honey"],
             "adjs": ["USDA Organic", "Non-GMO", "Raw", "Cold-Pressed", "Pure"],
             "brands": ["OrganicRoots", "EarthFirst", "PureHarvest", "GreenField", "NaturePure"]},
            {"name": "Beverages", "slug": "beverages", "price": (1.49, 39.99),
             "nouns": ["Sparkling Water", "Protein Shake", "Energy Drink", "Juice", "Kombucha", "Smoothie"],
             "adjs": ["Sugar-Free", "Zero-Calorie", "Vitamin-Enriched", "Natural", "Carbonated"],
             "brands": ["HydroBoost", "VitaSip", "RefreshCo", "DrinkWell", "BubbleX"]},
        ]
    },
    # ── Health & Wellness ─────────────────────────────────────────────────────
    {
        "root": "Health & Wellness", "root_slug": "health-wellness",
        "leaves": [
            {"name": "Vitamins & Supplements", "slug": "vitamins", "price": (9.99, 79.99),
             "nouns": ["Vitamin C", "Omega-3", "Multivitamin", "Probiotic", "Collagen", "Zinc"],
             "adjs": ["1000mg", "High-Potency", "Vegan", "Time-Release", "Third-Party Tested"],
             "brands": ["VitaCore", "HealthPlus", "NutriFit", "PureWell", "SupplementX"]},
            {"name": "Medical Devices", "slug": "medical-devices", "price": (19.99, 299.99),
             "nouns": ["Blood Pressure Monitor", "Thermometer", "Pulse Oximeter", "Glucose Meter", "Nebulizer"],
             "adjs": ["Digital", "Automatic", "Clinical-Grade", "FDA-Cleared", "Wireless"],
             "brands": ["MediTech", "HealthTrack", "ClinicalX", "VitalScan", "MedDevice"]},
            {"name": "First Aid", "slug": "first-aid", "price": (4.99, 59.99),
             "nouns": ["First Aid Kit", "Bandages", "Antiseptic Spray", "Medical Tape", "Ice Pack", "Gauze"],
             "adjs": ["240-Piece", "Waterproof", "Sterile", "Portable", "Emergency"],
             "brands": ["SafeGuard", "FirstCare", "MedKit", "HealFast", "AidPro"]},
            {"name": "Personal Care", "slug": "personal-care-health", "price": (3.99, 49.99),
             "nouns": ["Electric Toothbrush", "Floss", "Mouthwash", "Nail Clipper Set", "Facial Steamer"],
             "adjs": ["Sonic", "Whitening", "Travel-Size", "Rechargeable", "Professional"],
             "brands": ["OralCare Pro", "DentaX", "CleanSmile", "NailPerfect", "SteamFace"]},
        ]
    },
    # ── Jewelry & Watches ─────────────────────────────────────────────────────
    {
        "root": "Jewelry & Watches", "root_slug": "jewelry-watches",
        "leaves": [
            {"name": "Women's Jewelry", "slug": "womens-jewelry", "price": (14.99, 499.99),
             "nouns": ["Necklace", "Bracelet", "Earrings", "Ring", "Anklet", "Pendant"],
             "adjs": ["Sterling Silver", "Rose Gold", "Diamond-Cut", "Minimalist", "Boho"],
             "brands": ["LuxGems", "ShimmerCo", "GoldAura", "JewelCraft", "SilverMuse"]},
            {"name": "Watches", "slug": "watches", "price": (49.99, 999.99),
             "nouns": ["Chronograph", "Smartwatch", "Dive Watch", "Dress Watch", "Sport Watch"],
             "adjs": ["Automatic", "Quartz", "Waterproof", "Sapphire Crystal", "Swiss-Movement"],
             "brands": ["TimeCraft", "ChronoX", "WristMaster", "EliteTime", "PrecisionWatch"]},
            {"name": "Men's Jewelry", "slug": "mens-jewelry", "price": (19.99, 299.99),
             "nouns": ["Bracelet", "Ring", "Chain Necklace", "Cufflinks", "Dog Tag"],
             "adjs": ["Stainless Steel", "Titanium", "Matte Black", "Engraved", "Minimalist"],
             "brands": ["ManMetal", "BoldGear", "SteelCraft", "UrbanJewel", "RawEdge"]},
        ]
    },
    # ── Musical Instruments ───────────────────────────────────────────────────
    {
        "root": "Musical Instruments", "root_slug": "musical-instruments",
        "leaves": [
            {"name": "Guitars", "slug": "guitars", "price": (79.99, 1499.99),
             "nouns": ["Acoustic Guitar", "Electric Guitar", "Bass Guitar", "Classical Guitar", "Travel Guitar"],
             "adjs": ["Solid-Top", "Mahogany", "Rosewood", "Sunburst", "Beginner-Friendly"],
             "brands": ["StringMaster", "ChordCraft", "MelodyX", "FretBoard", "SoundWave"]},
            {"name": "Keyboards & Pianos", "slug": "keyboards-pianos", "price": (99.99, 1999.99),
             "nouns": ["Digital Piano", "MIDI Keyboard", "Synthesizer", "Stage Piano", "Arranger Keyboard"],
             "adjs": ["88-Key", "Weighted", "Semi-Weighted", "Bluetooth", "Portable"],
             "brands": ["KeyMaster", "PianoX", "SynthPro", "DigitalKeys", "IvoryTouch"]},
            {"name": "Drums & Percussion", "slug": "drums-percussion", "price": (49.99, 999.99),
             "nouns": ["Electronic Drum Kit", "Snare Drum", "Djembe", "Cajon", "Drum Pad", "Bongo Set"],
             "adjs": ["8-Piece", "Mesh-Head", "Professional", "Practice", "Acoustic"],
             "brands": ["BeatCraft", "RhythmX", "DrumMaster", "PercussionPro", "StickHit"]},
            {"name": "Studio Equipment", "slug": "studio-equipment", "price": (29.99, 799.99),
             "nouns": ["Audio Interface", "Studio Monitor", "Microphone", "Headphones", "MIDI Controller", "Pop Filter"],
             "adjs": ["XLR", "USB", "Condenser", "Dynamic", "Studio-Grade"],
             "brands": ["StudioX", "RecordPro", "AudioCraft", "SoundBooth", "MixMaster"]},
        ]
    },
    # ── Office Supplies ───────────────────────────────────────────────────────
    {
        "root": "Office Supplies", "root_slug": "office-supplies",
        "leaves": [
            {"name": "Stationery", "slug": "stationery", "price": (1.99, 39.99),
             "nouns": ["Ballpoint Pen", "Notebook", "Highlighter Set", "Sticky Notes", "Marker", "Planner"],
             "adjs": ["Gel-Ink", "Refillable", "Ruled", "Dotted", "Hardcover"],
             "brands": ["InkFlow", "PaperCraft", "WriteWell", "NoteX", "PenMaster"]},
            {"name": "Desk Accessories", "slug": "desk-accessories", "price": (9.99, 149.99),
             "nouns": ["Desk Organizer", "Monitor Stand", "Lamp", "Mouse Pad", "Cable Manager", "Stapler"],
             "adjs": ["Bamboo", "Adjustable", "Ergonomic", "LED", "Wireless-Charging"],
             "brands": ["DeskPro", "WorkSpace", "OfficeX", "OrganizerPlus", "DeskFlow"]},
            {"name": "Storage & Organization", "slug": "office-storage", "price": (14.99, 199.99),
             "nouns": ["File Cabinet", "Binder", "Label Maker", "Drawer Organizer", "Document Box", "Shelf"],
             "adjs": ["Lockable", "Stackable", "Color-Coded", "Heavy-Duty", "Letter-Size"],
             "brands": ["FileMax", "StorePro", "LabelCo", "OrderX", "ArchivePro"]},
            {"name": "Printers & Ink", "slug": "printers-ink", "price": (9.99, 299.99),
             "nouns": ["Ink Cartridge", "Toner Cartridge", "Label Printer", "Inkjet Printer", "Paper Ream"],
             "adjs": ["Compatible", "OEM", "High-Yield", "Photo-Quality", "Wireless"],
             "brands": ["PrintPro", "InkSaver", "TonerX", "CartridgePlus", "PrintMaster"]},
        ]
    },
    # ── Pets ──────────────────────────────────────────────────────────────────
    {
        "root": "Pets", "root_slug": "pets",
        "leaves": [
            {"name": "Dog Supplies", "slug": "dog-supplies", "price": (4.99, 199.99),
             "nouns": ["Dog Food", "Leash", "Harness", "Dog Bed", "Chew Toy", "Dog Crate"],
             "adjs": ["Grain-Free", "Adjustable", "Orthopedic", "Indestructible", "Reflective"],
             "brands": ["PawsFirst", "DogWorld", "TailWag", "WoofCo", "PetPro"]},
            {"name": "Cat Supplies", "slug": "cat-supplies", "price": (4.99, 149.99),
             "nouns": ["Cat Food", "Cat Litter", "Scratching Post", "Cat Tree", "Litter Box", "Cat Toy"],
             "adjs": ["Self-Cleaning", "Odor-Control", "Sisal", "Multi-Level", "Interactive"],
             "brands": ["PurrFect", "MeowCo", "CatLux", "WhiskerX", "FelinePro"]},
            {"name": "Fish & Aquarium", "slug": "fish-aquarium", "price": (7.99, 299.99),
             "nouns": ["Aquarium Tank", "Filter", "Heater", "Fish Food", "Gravel", "LED Light"],
             "adjs": ["20-Gallon", "Freshwater", "Saltwater", "Submersible", "Quiet-Flow"],
             "brands": ["AquaPro", "FishTank", "WaterWorld", "ClearStream", "AquaLife"]},
            {"name": "Bird Supplies", "slug": "bird-supplies", "price": (9.99, 129.99),
             "nouns": ["Bird Cage", "Bird Food", "Perch", "Nest Box", "Swing Toy", "Bird Bath"],
             "adjs": ["Stainless Steel", "Spacious", "Easy-Clean", "Natural Wood", "Colorful"],
             "brands": ["WingPro", "BirdHome", "FeatherX", "AvianCare", "NestMate"]},
        ]
    },
    # ── Tools & Home Improvement ──────────────────────────────────────────────
    {
        "root": "Tools & Home Improvement", "root_slug": "tools-home",
        "leaves": [
            {"name": "Power Tools", "slug": "power-tools", "price": (29.99, 599.99),
             "nouns": ["Drill", "Circular Saw", "Jigsaw", "Angle Grinder", "Random Sander", "Router"],
             "adjs": ["20V", "Brushless", "Cordless", "Variable-Speed", "Heavy-Duty"],
             "brands": ["PowerMax", "DrillPro", "ToolMaster", "WorkForce", "BuildX"]},
            {"name": "Hand Tools", "slug": "hand-tools", "price": (7.99, 149.99),
             "nouns": ["Hammer", "Screwdriver Set", "Wrench Set", "Pliers", "Level", "Utility Knife"],
             "adjs": ["Chrome-Vanadium", "Ergonomic", "Magnetic", "Professional-Grade", "Anti-Slip"],
             "brands": ["GripPro", "ToolCraft", "HandMax", "WorkBench", "SteelTool"]},
            {"name": "Electrical", "slug": "electrical", "price": (4.99, 249.99),
             "nouns": ["Extension Cord", "Wire Stripper", "Multimeter", "Circuit Breaker", "LED Bulb", "Outlet"],
             "adjs": ["Heavy-Duty", "Smart", "Surge-Protected", "12-Gauge", "Waterproof"],
             "brands": ["ElectroPro", "WireMaster", "VoltX", "CircuitSafe", "PowerLine"]},
            {"name": "Plumbing", "slug": "plumbing", "price": (4.99, 199.99),
             "nouns": ["Pipe Wrench", "Plunger", "Pipe Tape", "Faucet", "Drain Cleaner", "Valve"],
             "adjs": ["Heavy-Duty", "Compression-Fit", "Lead-Free", "Flexible", "Rust-Proof"],
             "brands": ["PipeMax", "FlowPro", "DrainMaster", "PlumbX", "WaterSeal"]},
        ]
    },
    # ── Travel & Luggage ──────────────────────────────────────────────────────
    {
        "root": "Travel & Luggage", "root_slug": "travel-luggage",
        "leaves": [
            {"name": "Suitcases", "slug": "suitcases", "price": (49.99, 399.99),
             "nouns": ["Carry-On", "Checked Suitcase", "Spinner Luggage", "Hard-Shell Case", "Luggage Set"],
             "adjs": ["Lightweight", "TSA-Approved", "Expandable", "360-Spinner", "Polycarbonate"],
             "brands": ["TravelPro", "JourneyX", "LuggagePlus", "RoamCo", "GlobeTrekker"]},
            {"name": "Backpacks & Bags", "slug": "travel-backpacks", "price": (29.99, 249.99),
             "nouns": ["Travel Backpack", "Laptop Bag", "Duffel Bag", "Tote Bag", "Drawstring Bag", "Messenger Bag"],
             "adjs": ["Water-Resistant", "Anti-Theft", "30L", "Convertible", "Slim-Profile"],
             "brands": ["PackPro", "BagMaster", "CarryX", "TravelGear", "BackpackCo"]},
            {"name": "Travel Accessories", "slug": "travel-accessories", "price": (4.99, 79.99),
             "nouns": ["Travel Pillow", "Packing Cubes", "Luggage Tag", "Travel Adapter", "Neck Wallet"],
             "adjs": ["Memory Foam", "Compression", "RFID-Blocking", "Universal", "Foldable"],
             "brands": ["TravelX", "PackLight", "JetSet", "RoamSafe", "ComfortTravel"]},
        ]
    },
    # ── Video Games ───────────────────────────────────────────────────────────
    {
        "root": "Video Games", "root_slug": "video-games",
        "leaves": [
            {"name": "PlayStation", "slug": "playstation", "price": (9.99, 79.99),
             "nouns": ["PS5 Game", "PS4 Game", "DualSense Controller", "PS5 Headset", "PS Plus Card"],
             "adjs": ["Action", "RPG", "Open-World", "Multiplayer", "Exclusive"],
             "brands": ["SonyGames", "PlaystationX", "PS Studio", "GameVault", "DigitalPlay"]},
            {"name": "Xbox", "slug": "xbox", "price": (9.99, 79.99),
             "nouns": ["Xbox Game", "Xbox Controller", "Game Pass Card", "Headset", "Charging Dock"],
             "adjs": ["4K", "FPS", "Strategy", "Exclusive", "Cross-Play"],
             "brands": ["XboxStore", "MicrosoftGames", "GamePass", "XboxPro", "GameHub"]},
            {"name": "Nintendo Switch", "slug": "nintendo-switch", "price": (9.99, 79.99),
             "nouns": ["Switch Game", "Joy-Con", "Switch Case", "Screen Protector", "Pro Controller"],
             "adjs": ["Family-Friendly", "Adventure", "Puzzle", "Portable", "Co-op"],
             "brands": ["NintendoX", "SwitchPro", "GameBridge", "NintenStore", "HanHeld"]},
            {"name": "PC Gaming", "slug": "pc-gaming", "price": (19.99, 199.99),
             "nouns": ["Gaming Mouse", "Mechanical Keyboard", "Gaming Headset", "Mouse Pad", "PC Game Key"],
             "adjs": ["RGB", "High-DPI", "Wireless", "Surround-Sound", "Programmable"],
             "brands": ["GameGear", "PCMaster", "RGBpro", "ClickMax", "FPSking"]},
        ]
    },
]


def _leaf_categories():
    """Flatten TAXONOMY into a list of leaf dicts with root metadata."""
    leaves = []
    for group in TAXONOMY:
        for leaf in group["leaves"]:
            leaves.append({**leaf, "_root": group["root"], "_root_slug": group["root_slug"]})
    return leaves


LEAF_CATEGORIES = _leaf_categories()


# ── Seeding functions ──────────────────────────────────────────────────────────

def seed_sellers(users_conn, dry_run: bool) -> list[str]:
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
    return [s["id"] for s in SELLERS]


def seed_categories(prod_conn, dry_run: bool) -> dict:
    """Insert new root + leaf categories, return slug→id mapping for all leaves."""
    cat_id_map = {}

    # Insert root categories
    root_slugs = {}
    for group in TAXONOMY:
        if not dry_run:
            prod_conn.execute(
                """
                INSERT INTO categories (name, slug, parent_id, sort_order)
                VALUES (%s, %s, NULL, %s)
                ON CONFLICT (slug) DO NOTHING
                """,
                (group["root"], group["root_slug"], len(root_slugs) + 13),
            )
        root_slugs[group["root_slug"]] = group

    # Fetch all root IDs (including pre-existing ones from V2)
    rows = prod_conn.execute(
        "SELECT slug, id FROM categories WHERE parent_id IS NULL"
    ).fetchall()
    root_id_by_slug = {r[0]: r[1] for r in rows}

    # Insert leaf categories
    sort_order = 0
    for group in TAXONOMY:
        root_id = root_id_by_slug.get(group["root_slug"])
        for leaf in group["leaves"]:
            sort_order += 1
            if not dry_run:
                prod_conn.execute(
                    """
                    INSERT INTO categories (name, slug, parent_id, sort_order)
                    VALUES (%s, %s, %s, %s)
                    ON CONFLICT (slug) DO NOTHING
                    """,
                    (leaf["name"], leaf["slug"], root_id, sort_order),
                )

    if not dry_run:
        prod_conn.commit()

    # Fetch all leaf IDs
    rows = prod_conn.execute(
        "SELECT slug, id FROM categories WHERE parent_id IS NOT NULL"
    ).fetchall()
    for slug, cid in rows:
        cat_id_map[slug] = cid

    return cat_id_map


def _make_name(leaf: dict) -> str:
    brand = random.choice(leaf["brands"])
    adj = random.choice(leaf["adjs"])
    noun = random.choice(leaf["nouns"])
    # Occasionally add a suffix for variety
    suffix = random.choice(["", " Pro", " Plus", " Lite", " Max", " X", " 2.0", " Premium", ""])
    return f"{brand} {adj} {noun}{suffix}"


def _make_description(leaf: dict) -> str:
    noun = random.choice(leaf["nouns"])
    adj1 = random.choice(leaf["adjs"])
    adj2 = random.choice(leaf["adjs"])
    cat_name = leaf["name"]
    options = [
        f"{adj1} {noun} for everyday use. Perfect for {cat_name} enthusiasts. {adj2} design with premium materials.",
        f"High-quality {noun} featuring {adj1} technology. Ideal for {cat_name}. Includes {adj2} finish.",
        f"Professional-grade {noun}. {adj1} performance meets {adj2} durability. Trusted by {cat_name} professionals.",
        f"Compact {adj1} {noun} with {adj2} build quality. Best in class for {cat_name} needs.",
        f"{adj1} {noun} — redefining {cat_name}. {adj2} construction, long-lasting performance.",
    ]
    return random.choice(options)


def _make_price(leaf: dict) -> str:
    lo, hi = leaf["price"]
    return str(round(random.uniform(lo, hi), 2))


def _make_status(i: int) -> str:
    if i % 20 == 0:
        return "DELETED"
    if i % 10 == 0:
        return "INACTIVE"
    return "ACTIVE"


def seed_products(prod_conn, cat_id_map: dict, seller_ids: list, target: int, dry_run: bool) -> int:
    """Stream 100K product rows via COPY. Returns the minimum inserted product id."""
    if dry_run:
        print(f"  [dry-run] would insert {target} products")
        return -1

    # Map leaf slugs that exist in cat_id_map
    available_leaves = [lf for lf in LEAF_CATEGORIES if lf["slug"] in cat_id_map]
    if not available_leaves:
        raise RuntimeError("No leaf categories found in DB — run seed_categories first.")

    seller_cycle = itertools.cycle(seller_ids)
    leaf_cycle = itertools.cycle(available_leaves)

    # Get current max product id
    row = prod_conn.execute("SELECT COALESCE(MAX(id), 0) FROM products").fetchone()
    id_before = row[0]

    with prod_conn.cursor().copy(
        "COPY products (name, description, price, category_id, seller_id, status, stock_quantity, stock_reserved, version) FROM STDIN"
    ) as copy:
        for i in range(1, target + 1):
            leaf = next(leaf_cycle)
            cat_id = cat_id_map[leaf["slug"]]
            seller_id = next(seller_cycle)
            status = _make_status(i)
            stock_qty = random.randint(0, 500)
            stock_reserved = random.randint(0, min(10, stock_qty)) if status == "ACTIVE" else 0
            copy.write_row((
                _make_name(leaf),
                _make_description(leaf),
                _make_price(leaf),
                cat_id,
                seller_id,
                status,
                stock_qty,
                stock_reserved,
                0,
            ))

    prod_conn.commit()

    row = prod_conn.execute("SELECT COALESCE(MIN(id), 0) FROM products WHERE id > %s", (id_before,)).fetchone()
    return row[0]


def seed_images(prod_conn, min_product_id: int, dry_run: bool):
    if dry_run or min_product_id <= 0:
        return

    ids = prod_conn.execute(
        "SELECT id, name FROM products WHERE id >= %s ORDER BY id", (min_product_id,)
    ).fetchall()

    with prod_conn.cursor().copy(
        "COPY product_images (product_id, url, alt_text, sort_order) FROM STDIN"
    ) as copy:
        for pid, name in ids:
            # Primary image for every product
            copy.write_row((pid, f"https://picsum.photos/seed/p{pid}/600/400", f"{name[:80]} — main view", 0))
            # Secondary image for ~50% of products
            if pid % 2 == 0:
                copy.write_row((pid, f"https://picsum.photos/seed/p{pid}b/600/400", f"{name[:80]} — side view", 1))

    prod_conn.commit()
    return len(ids)


# ── Main ───────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Seed sellers, categories, and products.")
    parser.add_argument("--target", type=int, default=100_000, help="Number of products to insert (default: 100000)")
    parser.add_argument("--skip-sellers", action="store_true", help="Skip seller user seeding")
    parser.add_argument("--skip-images", action="store_true", help="Skip product_images seeding")
    parser.add_argument("--dry-run", action="store_true", help="Count only — no writes")
    args = parser.parse_args()

    random.seed(42)
    t0 = time.time()

    print(f"{'[DRY RUN] ' if args.dry_run else ''}Connecting to databases...")
    users_conn = psycopg.connect(USERS_DSN)
    prod_conn = psycopg.connect(PRODUCTS_DSN)

    # 1. Sellers
    if not args.skip_sellers:
        print(f"Seeding {len(SELLERS)} sellers into ecommerce_users...", end=" ", flush=True)
        seller_ids = seed_sellers(users_conn, args.dry_run)
        print(f"done ({len(seller_ids)} sellers)")
    else:
        seller_ids = [s["id"] for s in SELLERS]
        print(f"Skipping sellers — using {len(seller_ids)} pre-defined UUIDs")

    # 2. Categories
    print("Seeding categories...", end=" ", flush=True)
    cat_id_map = seed_categories(prod_conn, args.dry_run)
    print(f"done ({len(cat_id_map)} leaf categories mapped)")

    # 3. Products
    print(f"Seeding {args.target:,} products via COPY...", end=" ", flush=True)
    t_prod = time.time()
    min_id = seed_products(prod_conn, cat_id_map, seller_ids, args.target, args.dry_run)
    elapsed_prod = time.time() - t_prod
    print(f"done in {elapsed_prod:.1f}s")

    # 4. Images
    if not args.skip_images:
        print("Seeding product images via COPY...", end=" ", flush=True)
        t_img = time.time()
        img_count = seed_images(prod_conn, min_id, args.dry_run)
        elapsed_img = time.time() - t_img
        if img_count:
            print(f"done in {elapsed_img:.1f}s ({img_count + img_count // 2:,} images)")
        else:
            print("skipped (dry-run or no new products)")

    users_conn.close()
    prod_conn.close()

    total = time.time() - t0
    print(f"\nTotal: {args.target:,} products seeded in {total:.1f}s")

    if not args.dry_run and not args.skip_sellers:
        print(f"\nSeller accounts (password: {SELLER_PASSWORD}):")
        for s in SELLERS:
            print(f"  {s['email']:<35} → {s['id']}")

    print("\nNext step — generate embeddings:")
    print("  docker exec -it ecommerce-ai-service python scripts/embed_products.py")


if __name__ == "__main__":
    main()
