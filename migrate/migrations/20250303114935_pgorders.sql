-- +goose Up
-- +goose StatementBegin
CREATE TABLE order_states (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE package_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE wrapper_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE orders (
    id BIGINT PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    state_id INTEGER NOT NULL REFERENCES order_states(id),
    weight DECIMAL(10, 2) NOT NULL,
    cost DECIMAL(10, 2) NOT NULL,
    package_type_id INTEGER REFERENCES package_types(id),
    wrapper_type_id INTEGER REFERENCES wrapper_types(id),
    deadline_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delivered_at TIMESTAMP WITH TIME ZONE,
    returned_at TIMESTAMP WITH TIME ZONE
);

INSERT INTO order_states (name) VALUES 
    ('accepted'), 
    ('delivered'), 
    ('returned');

INSERT INTO package_types (name) VALUES 
    ('bag'), 
    ('box'), 
    ('film');

INSERT INTO wrapper_types (name) VALUES 
    ('film');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE orders;
DROP TABLE wrapper_types;
DROP TABLE package_types;
DROP TABLE order_states;
-- +goose StatementEnd
