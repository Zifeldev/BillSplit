-- +goose Up
-- Global categorization rules for Kazakhstan (Freedom Bank, Kaspi...) merchants
-- and statement texts. Patterns are lower-case substrings of "merchant description";
-- longer patterns win, user rules always come first.
-- Not added on purpose: bare 'small' (Small supermarkets) — it matches too many
-- unrelated English descriptors; 'activ' — matches "active".
INSERT INTO category_rules (user_id, pattern, category_id) VALUES
    -- own accounts, person-to-person and currency exchange
    (NULL, 'между своими счетами', 'transfer'),
    (NULL, 'безвозмездный перевод', 'transfer'),
    (NULL, 'безвозмездные переводы', 'transfer'),
    (NULL, 'перевод с карты на карту', 'transfer'),
    (NULL, 'перевод валюты', 'transfer'),
    (NULL, 'пополнение через банкомат', 'transfer'),
    (NULL, 'kaspi', 'transfer'),
    (NULL, 'каспи', 'transfer'),
    (NULL, 'выплата процентов', 'income'),
    -- groceries
    (NULL, 'magnum', 'groceries'),
    (NULL, 'galmart', 'groceries'),
    (NULL, 'галмарт', 'groceries'),
    (NULL, 'small market', 'groceries'),
    (NULL, 'arbuz.kz', 'groceries'),
    -- transport
    (NULL, 'yandex go', 'transport'),
    (NULL, 'yandex.go', 'transport'),
    (NULL, 'яндекс go', 'transport'),
    (NULL, 'yandex taxi', 'transport'),
    (NULL, 'яндекс такси', 'transport'),
    (NULL, 'indrive', 'transport'),
    -- dining
    (NULL, 'glovo', 'dining'),
    (NULL, 'wolt', 'dining'),
    (NULL, 'yandex eda', 'dining'),
    (NULL, 'яндекс еда', 'dining'),
    -- utilities / telecom
    (NULL, 'beeline', 'utilities'),
    (NULL, 'kcell', 'utilities'),
    (NULL, 'tele2', 'utilities'),
    (NULL, 'altel', 'utilities'),
    (NULL, 'kazakhtelecom', 'utilities'),
    (NULL, 'казахтелеком', 'utilities'),
    -- travel
    (NULL, 'aviata', 'travel'),
    (NULL, 'chocotravel', 'travel'),
    (NULL, 'air astana', 'travel')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM category_rules WHERE user_id IS NULL AND pattern IN (
    'между своими счетами', 'безвозмездный перевод', 'безвозмездные переводы', 'перевод с карты на карту',
    'перевод валюты', 'пополнение через банкомат', 'kaspi', 'каспи', 'выплата процентов',
    'magnum', 'galmart', 'галмарт', 'small market', 'arbuz.kz',
    'yandex go', 'yandex.go', 'яндекс go', 'yandex taxi', 'яндекс такси', 'indrive',
    'glovo', 'wolt', 'yandex eda', 'яндекс еда',
    'beeline', 'kcell', 'tele2', 'altel', 'kazakhtelecom', 'казахтелеком',
    'aviata', 'chocotravel', 'air astana');
