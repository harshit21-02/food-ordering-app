-- Dev seed — Tealogy Cafe, Raipur Khadar (Noida) — taken from the printed menu.
-- Re-runnable on a freshly migrated DB: run after `make migrate-up` (or `make migrate-reset`).

-- 1. Organisation -------------------------------------------------------------
INSERT INTO organisations (name, address, contact_phone, contact_email)
VALUES (
    'Tealogy Cafe — Raipur Khadar',
    'Raipur Khadar, Opp. Amity University Gate No-2, Sector 126, Noida, UP - 201301',
    '+917351600408',
    'franchise@tealogy.in'
);

-- 2. Staff: super admin + branch admin + branch staff -------------------------
-- All three log in via email + OTP at /admin/login. password_hash is NULL
-- (we don't use passwords). For the demo both roles re-use the same Gmail
-- inbox via plus-addressing — Gmail delivers every "name+anything@gmail.com"
-- to the "name@gmail.com" inbox, so one mailbox can sign in as either role.
INSERT INTO staff_users (org_id, email, mobile_number, name, role) VALUES
    (NULL, 'prateekpal641@gmail.com',            '+919999000001', 'Platform Super Admin',   'super_admin'),
    (1,    'noreply.tealogy@gmail.com',         '+917351600408', 'Tealogy Branch Admin',   'manager'),
    (1,    'noreply.tealogy+staff@gmail.com',   '+919876500003', 'Tealogy Staff',          'staff');

-- 3. Tables (5 — codes are short opaque ids that go into the QR URL) ----------
INSERT INTO tables (org_id, code, label) VALUES
    (1, 't_a7Kx9', 'Table 1'),
    (1, 't_b2Mn4', 'Table 2'),
    (1, 't_c5Pq7', 'Table 3'),
    (1, 't_d8Rs2', 'Table 4'),
    (1, 't_e1Tu5', 'Table 5');

-- 3. Menu ---------------------------------------------------------------------
-- Items with two size variants (chai/tea/coffee) get one row per size.
-- Categories/order match the printed menu.

-- DESI CHAI
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Cutting Chai (Paper / Small Kulhad)',     'Desi Chai', 20.00, 1),
    (1, 'Cutting Chai (Medium Kulhad)',            'Desi Chai', 39.00, 2),
    (1, 'Adrak Tea (Paper / Small Kulhad)',        'Desi Chai', 20.00, 3),
    (1, 'Adrak Tea (Medium Kulhad)',               'Desi Chai', 39.00, 4),
    (1, 'Elaichi Tea (Paper / Small Kulhad)',      'Desi Chai', 20.00, 5),
    (1, 'Elaichi Tea (Medium Kulhad)',             'Desi Chai', 39.00, 6),
    (1, 'Masala Tea (Paper / Small Kulhad)',       'Desi Chai', 20.00, 7),
    (1, 'Masala Tea (Medium Kulhad)',              'Desi Chai', 39.00, 8),
    (1, 'Gud Wali Chai (Paper / Small Kulhad)',    'Desi Chai', 20.00, 9),
    (1, 'Gud Wali Chai (Medium Kulhad)',           'Desi Chai', 39.00, 10),
    (1, 'Baarish Wali Chai (Paper / Small Kulhad)','Desi Chai', 20.00, 11),
    (1, 'Baarish Wali Chai (Medium Kulhad)',       'Desi Chai', 39.00, 12);

-- FLAVOURED TEA
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Chocolate Tea (Paper / Small Kulhad)',    'Flavoured Tea', 20.00, 1),
    (1, 'Chocolate Tea (Medium Kulhad)',           'Flavoured Tea', 39.00, 2),
    (1, 'Paan Tea (Paper / Small Kulhad)',         'Flavoured Tea', 20.00, 3),
    (1, 'Paan Tea (Medium Kulhad)',                'Flavoured Tea', 39.00, 4),
    (1, 'Kesar Tea (Paper / Small Kulhad)',        'Flavoured Tea', 20.00, 5),
    (1, 'Kesar Tea (Medium Kulhad)',               'Flavoured Tea', 39.00, 6),
    (1, 'Rose Tea (Paper / Small Kulhad)',         'Flavoured Tea', 20.00, 7),
    (1, 'Rose Tea (Medium Kulhad)',                'Flavoured Tea', 39.00, 8),
    (1, 'Mango Tea (Paper / Small Kulhad)',        'Flavoured Tea', 20.00, 9),
    (1, 'Mango Tea (Medium Kulhad)',               'Flavoured Tea', 39.00, 10);

-- ICED TEA
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Lemon Mint Iced Tea', 'Iced Tea', 99.00, 1);

-- COFFEE
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Black Coffee (Medium Kulhad)',            'Coffee', 39.00, 1),
    (1, 'Hot Coffee (Paper / Small Kulhad)',       'Coffee', 29.00, 2),
    (1, 'Hot Coffee (Medium Kulhad)',              'Coffee', 49.00, 3),
    (1, 'Chocolate Hot Coffee (Paper / Small Kulhad)', 'Coffee', 39.00, 4),
    (1, 'Chocolate Hot Coffee (Medium Kulhad)',    'Coffee', 59.00, 5);

-- COLD COFFEE
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Cold Coffee',               'Cold Coffee', 79.00, 1),
    (1, 'Cold Coffee With Ice Cream','Cold Coffee', 89.00, 2),
    (1, 'Chocolate Cold Coffee',     'Cold Coffee', 99.00, 3);

-- MILK
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Badam Kesar Milk (Hot)', 'Milk', 99.00, 1);

-- ICE CRUSH
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Kiwi Ice Crush',       'Ice Crush', 99.00, 1),
    (1, 'Blueberry Ice Crush',  'Ice Crush', 99.00, 2),
    (1, 'Strawberry Ice Crush', 'Ice Crush', 99.00, 3);

-- CHEESECAKE MILK SHAKE
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Mango Cheesecake Shake',      'Cheesecake Milk Shake', 149.00, 1),
    (1, 'Strawberry Cheesecake Shake', 'Cheesecake Milk Shake', 149.00, 2),
    (1, 'Blueberry Cheesecake Shake',  'Cheesecake Milk Shake', 149.00, 3);

-- SHAKES
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Chocolate Shake', 'Shakes',  99.00, 1),
    (1, 'Oreo Shake',      'Shakes',  99.00, 2),
    (1, 'Kit Kat Shake',   'Shakes',  99.00, 3),
    (1, 'Brownie Shake',   'Shakes', 129.00, 4);

-- FRUIT SHAKES
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Kiwi Shake',               'Fruit Shakes',  99.00, 1),
    (1, 'Mango Shake',              'Fruit Shakes',  99.00, 2),
    (1, 'Blueberry Shake',          'Fruit Shakes',  99.00, 3),
    (1, 'Strawberry Shake',         'Fruit Shakes',  99.00, 4),
    (1, 'Chiku Milkshake',          'Fruit Shakes', 129.00, 5),
    (1, 'Kesar Thandaai Milkshake', 'Fruit Shakes', 129.00, 6);

-- COOLERS
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Classic Jeeru',   'Coolers', 89.00, 1),
    (1, 'Masala Lemonade', 'Coolers', 89.00, 2),
    (1, 'Shikanji',        'Coolers', 89.00, 3),
    (1, 'Chilli Guava',    'Coolers', 89.00, 4);

-- TEA-SNACKS
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Butter Toast',          'Tea-Snacks',  49.00, 1),
    (1, 'Butter Maska Bun',      'Tea-Snacks',  59.00, 2),
    (1, 'French Fries',          'Tea-Snacks',  89.00, 3),
    (1, 'Potato Shots',          'Tea-Snacks',  99.00, 4),
    (1, 'Peri Peri French Fries','Tea-Snacks', 109.00, 5);

-- SANDWICH
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Coleslaw Sandwich',         'Sandwich',  79.00, 1),
    (1, 'Masala Sandwich',           'Sandwich',  89.00, 2),
    (1, 'Cheese Chutney Sandwich',   'Sandwich', 119.00, 3),
    (1, 'Cheese Corn Sandwich',      'Sandwich', 119.00, 4),
    (1, 'Chocolate Sandwich',        'Sandwich', 119.00, 5),
    (1, 'Vegetable Club Sandwich',   'Sandwich', 119.00, 6);

-- MAGGI
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Masala Maggi',     'Maggi', 69.00, 1),
    (1, 'Vegetable Maggi',  'Maggi', 79.00, 2),
    (1, 'Cheese Corn Maggi','Maggi', 99.00, 3);

-- DESI
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Vada Pav',           'Desi', 39.00, 1),
    (1, 'Dabeli',             'Desi', 39.00, 2),
    (1, 'Indori Poha',        'Desi', 59.00, 3),
    (1, 'Dry Fruit Upma Mix', 'Desi', 69.00, 4),
    (1, 'Masala Oats Meal',   'Desi', 69.00, 5);

-- MOMOS (each item has Half / Full portions — separate rows)
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Veg Momos (Half — 6 pcs)',    'Momos',  89.00, 1),
    (1, 'Veg Momos (Full — 8 pcs)',    'Momos', 129.00, 2),
    (1, 'Paneer Momos (Half — 6 pcs)', 'Momos', 109.00, 3),
    (1, 'Paneer Momos (Full — 8 pcs)', 'Momos', 139.00, 4);

-- CORN
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Salted Corn',   'Corn', 49.00, 1),
    (1, 'Italian Corn',  'Corn', 79.00, 2),
    (1, 'Peri Peri Corn','Corn', 89.00, 3),
    (1, 'Cheese Corn',   'Corn', 89.00, 4);

-- BURGER
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Aloo Tikki Burger', 'Burger',  49.00, 1),
    (1, 'Masala Burger',     'Burger',  79.00, 2),
    (1, 'Veg Burger',        'Burger',  89.00, 3),
    (1, 'Paneer Burger',     'Burger', 119.00, 4);

-- RICE BOWL (Meal Combo Add-on)
INSERT INTO menu (org_id, name, category, price, display_order) VALUES
    (1, 'Dal Chawal',                     'Rice Bowl', 119.00, 1),
    (1, 'Butter Khichdi',                 'Rice Bowl', 129.00, 2),
    (1, 'Rajma Rice',                     'Rice Bowl', 139.00, 3),
    (1, 'Dal Makhani With Rice',          'Rice Bowl', 149.00, 4),
    (1, 'Paneer Butter Masala With Rice', 'Rice Bowl', 159.00, 5);
