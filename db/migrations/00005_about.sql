-- +goose Up
-- About section is a singleton row (id = 1): two paragraphs plus a quote.
-- Seeded with the current hardcoded homepage text.
CREATE TABLE about (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  p1 TEXT NOT NULL,
  p2 TEXT NOT NULL,
  quote TEXT NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO about (id, p1, p2, quote) VALUES (
  1,
  'P.O.H je slovenská folk-rocková kapela, ktorá pochádza z Dolného Kubína a ktorej vznik nebol okamihom, ale procesom, takže jeho presné datovanie nie je uchopiteľné; pár rokov to však už bude… V zložení dve gitary, basgitara a bicie si prešla mnohými koncertmi, či už na klubových pódiách alebo menších i väčších festivaloch v SR a ČR.',
  'Na konte má štyri demo albumy: Posledný smiech, Stodola, Deň za dňom, Večnosť a jeden štúdiový Na konci sveta (2021). Hrá výsostne vlastnú produkciu a všetky jej texty sú v rodnej slovenčine, čo je pre kapelu úplne prirodzené a poslucháčmi (súdiac podľa ich ohlasov) oceňované.',
  'Obojstranný prúd energie na koncertoch je pre P.O.H na nezaplatenie – kto to zažil, ten vie…'
);

-- +goose Down
DROP TABLE IF EXISTS about;
