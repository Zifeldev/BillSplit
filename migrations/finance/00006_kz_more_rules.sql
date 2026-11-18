-- +goose Up
-- More global categorization rules for Kazakhstan merchants (BCC, Kaspi and
-- other KZ bank statements). Lower-case substrings of "merchant description";
-- longer patterns win, so 'kaspi магазин' (shopping) beats 'kaspi' (transfer).
INSERT INTO category_rules (user_id, pattern, category_id) VALUES
    -- cash: not spending yet
    (NULL, 'снятие наличных', 'transfer'),
    (NULL, 'cash withdrawal', 'transfer'),
    -- groceries
    (NULL, 'anvar', 'groceries'),
    (NULL, 'анвар', 'groceries'),
    (NULL, 'airba fresh', 'groceries'),
    (NULL, 'metro cash', 'groceries'),
    (NULL, 'interfood', 'groceries'),
    -- fuel & city transport
    (NULL, 'helios', 'transport'),
    (NULL, 'гелиос', 'transport'),
    (NULL, 'sinooil', 'transport'),
    (NULL, 'qazaq oil', 'transport'),
    (NULL, 'qazaqoil', 'transport'),
    (NULL, 'kazmunaygas', 'transport'),
    (NULL, 'казмунайгаз', 'transport'),
    (NULL, 'onay', 'transport'),
    -- travel
    (NULL, 'flyarystan', 'travel'),
    (NULL, 'fly arystan', 'travel'),
    (NULL, 'scat airlines', 'travel'),
    (NULL, 'temir zholy', 'travel'),
    (NULL, 'темир жолы', 'travel'),
    (NULL, 'tickets.kz', 'travel'),
    -- shopping
    (NULL, 'kaspi магазин', 'shopping'),
    (NULL, 'kaspi magazin', 'shopping'),
    (NULL, 'wildberries', 'shopping'),
    (NULL, 'ozon', 'shopping'),
    (NULL, 'technodom', 'shopping'),
    (NULL, 'технодом', 'shopping'),
    (NULL, 'sulpak', 'shopping'),
    (NULL, 'сулпак', 'shopping'),
    (NULL, 'mechta.kz', 'shopping'),
    (NULL, 'alser', 'shopping'),
    (NULL, 'lamoda', 'shopping'),
    (NULL, 'flip.kz', 'shopping'),
    (NULL, 'fix price', 'shopping'),
    -- health
    (NULL, 'europharma', 'health'),
    (NULL, 'еврофарма', 'health'),
    (NULL, 'аптека', 'health'),
    (NULL, 'apteka', 'health'),
    (NULL, 'sadyhan', 'health'),
    (NULL, 'садыхан', 'health'),
    (NULL, 'invivo', 'health'),
    -- dining
    (NULL, 'bahandi', 'dining'),
    (NULL, 'salam bro', 'dining'),
    (NULL, 'coffee boom', 'dining'),
    (NULL, 'del papa', 'dining'),
    (NULL, 'chocofood', 'dining'),
    -- entertainment & subscriptions
    (NULL, 'kinopark', 'entertainment'),
    (NULL, 'кинопарк', 'entertainment'),
    (NULL, 'chaplin', 'entertainment'),
    (NULL, 'yandex plus', 'subscriptions'),
    (NULL, 'яндекс плюс', 'subscriptions'),
    (NULL, 'kinopoisk', 'subscriptions'),
    (NULL, 'кинопоиск', 'subscriptions'),
    -- utilities
    (NULL, 'alseco', 'utilities'),
    (NULL, 'алсеко', 'utilities'),
    (NULL, 'астана-ерц', 'utilities')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM category_rules WHERE user_id IS NULL AND pattern IN (
    'снятие наличных', 'cash withdrawal',
    'anvar', 'анвар', 'airba fresh', 'metro cash', 'interfood',
    'helios', 'гелиос', 'sinooil', 'qazaq oil', 'qazaqoil', 'kazmunaygas', 'казмунайгаз', 'onay',
    'flyarystan', 'fly arystan', 'scat airlines', 'temir zholy', 'темир жолы', 'tickets.kz',
    'kaspi магазин', 'kaspi magazin', 'wildberries', 'ozon', 'technodom', 'технодом', 'sulpak', 'сулпак',
    'mechta.kz', 'alser', 'lamoda', 'flip.kz', 'fix price',
    'europharma', 'еврофарма', 'аптека', 'apteka', 'sadyhan', 'садыхан', 'invivo',
    'bahandi', 'salam bro', 'coffee boom', 'del papa', 'chocofood',
    'kinopark', 'кинопарк', 'chaplin', 'yandex plus', 'яндекс плюс', 'kinopoisk', 'кинопоиск',
    'alseco', 'алсеко', 'астана-ерц');
