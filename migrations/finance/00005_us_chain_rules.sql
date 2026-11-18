-- +goose Up
-- Common US chains seen on card statements (substring match on merchant/description).
INSERT INTO category_rules (user_id, pattern, category_id) VALUES
    -- restaurants & fast food
    (NULL, 'kfc', 'dining'), (NULL, 'subway', 'dining'), (NULL, 'raising cane', 'dining'),
    (NULL, 'burger king', 'dining'), (NULL, 'wendy''s', 'dining'), (NULL, 'taco bell', 'dining'),
    (NULL, 'panda express', 'dining'), (NULL, 'chick-fil-a', 'dining'), (NULL, 'popeyes', 'dining'),
    (NULL, 'five guys', 'dining'), (NULL, 'in-n-out', 'dining'), (NULL, 'dunkin', 'dining'),
    (NULL, 'domino', 'dining'), (NULL, 'pizza', 'dining'), (NULL, 'sushi', 'dining'),
    (NULL, 'sakana', 'dining'), (NULL, 'panera', 'dining'), (NULL, 'jack in the box', 'dining'),
    (NULL, 'sonic drive', 'dining'), (NULL, 'dutch bros', 'dining'), (NULL, 'restaurant', 'dining'),
    (NULL, 'cafe', 'dining'), (NULL, 'coffee', 'dining'), (NULL, 'grill', 'dining'),
    -- groceries & convenience
    (NULL, 'fry''s', 'groceries'), (NULL, 'frys food', 'groceries'), (NULL, 'sprouts', 'groceries'),
    (NULL, 'circle k', 'groceries'), (NULL, '7-eleven', 'groceries'), (NULL, 'on the go', 'groceries'),
    (NULL, 'dollar general', 'groceries'), (NULL, 'family dollar', 'groceries'),
    -- fuel
    (NULL, 'arco', 'transport'), (NULL, 'valero', 'transport'),
    (NULL, 'qt ', 'transport'), (NULL, 'quiktrip', 'transport'),
    -- shopping
    (NULL, 'burlington', 'shopping'), (NULL, 'ross stores', 'shopping'), (NULL, 'tj maxx', 'shopping'),
    (NULL, 'marshalls', 'shopping'), (NULL, 'ulta', 'shopping'), (NULL, 'nike', 'shopping'),
    (NULL, 'apple store', 'shopping'), (NULL, 'ebay', 'shopping'),
    -- subscriptions & software
    (NULL, 'upwork', 'subscriptions'), (NULL, 'openai', 'subscriptions'), (NULL, 'anthropic', 'subscriptions'),
    (NULL, 'google *', 'subscriptions'), (NULL, 'adobe', 'subscriptions'), (NULL, 'icloud', 'subscriptions')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM category_rules WHERE user_id IS NULL AND pattern IN (
    'kfc','subway','raising cane','burger king','wendy''s','taco bell','panda express','chick-fil-a','popeyes',
    'five guys','in-n-out','dunkin','domino','pizza','sushi','sakana','panera','jack in the box','sonic drive',
    'dutch bros','restaurant','cafe','coffee','grill','fry''s','frys food','sprouts','circle k','7-eleven',
    'on the go','dollar general','family dollar','arco','valero','qt ','quiktrip','burlington',
    'ross stores','tj maxx','marshalls','ulta','nike','apple store','ebay','upwork','openai','anthropic',
    'google *','adobe','icloud');
