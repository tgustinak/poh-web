BEGIN;

CREATE TEMP TABLE legacy_concerts (
    name TEXT NOT NULL,
    event_date TEXT NOT NULL
);

INSERT INTO legacy_concerts (name, event_date) VALUES
-- 2024
('Šikmá veža fest', '2024-06-08'),
('Hody Veľké Kostoľany', '2024-06-15'),
('Svätojánska vatra Dolný Kubín', '2024-06-22'),
('Jasenová Deň starostov', '2024-06-29'),
('Kúpele Sliač', '2024-07-19'),
('Hody Trebatice', '2024-08-17'),
('Súkromná akcia', '2024-09-06'),
('Rozlúčka s letom Most pri Bratislave', '2024-09-07'),
('Hody Častkovce', '2024-09-15'),
('Tvrdošín – Michalský jarmok', '2024-09-27'),
('Teplický festival vína a burčiaku (Trenčianske Teplice)', '2024-10-11'),
('Firemná akcia', '2024-11-15'),
('Darujme Vianoce 2024', '2024-12-13'),

-- 2025
('Hody Veľké Kostoľany', '2025-06-13'),
('Cífer', '2025-06-14'),
('Dlhá nad Oravou', '2025-06-21'),
('Jasenová', '2025-06-28'),
('Liptovský Mikuláš', '2025-07-11'),
('Tatranská Lomnica', '2025-07-15'),
('Kúpele Sliač', '2025-07-25'),
('Ratkovce', '2025-08-02'),
('Hody Trebatice', '2025-08-16'),
('Handlová', '2025-08-29'),
('Leštiny', '2025-08-31'),
('Hrabovo – súkromná akcia', '2025-09-14'),
('Rimavská Sobota', '2025-09-26'),
('Zlaté Moravce – Jarmok', '2025-09-27'),
('Vrbové', '2025-10-11'),
('Stará Ľubovňa', '2025-12-12'),

-- 2026
('On Stage, MsKS Dolný Kubín', '2026-04-10'),
('Laugaricio, Trenčín', '2026-05-01'),
('Sokolovce', '2026-06-12'),
('Drienčany', '2026-06-13'),
('Cífer', '2026-06-20'),
('Jasenová', '2026-06-27'),
('Kňažia', '2026-07-17'),
('Kynceľová', '2026-07-18'),
('Čierny Balog', '2026-07-18'),
('ŠpanRockFest, Špania Dolina', '2026-08-01'),
('Súkromná akcia', '2026-08-07'),
('Žabokreky', '2026-08-22'),
('Veľká Polom, Ostrava', '2026-08-29'),
('Madunice', '2026-09-05'),
('Ratkovce', '2026-09-05'),
('Kubínsky Jarmok', '2026-09-18'),
('Michalský jarmok, Ľubietová', '2026-09-26');

INSERT INTO concerts (name, event_date)
SELECT name, event_date
FROM legacy_concerts
WHERE NOT EXISTS (
    SELECT 1 FROM concerts
    WHERE
        concerts.name = legacy_concerts.name
        AND concerts.event_date = legacy_concerts.event_date
);

DROP TABLE legacy_concerts;

COMMIT;
