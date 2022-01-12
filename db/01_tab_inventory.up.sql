/* A function for updating the "updated_at" column during updates. */
CREATE OR REPLACE FUNCTION inventory_updated_at ()
    RETURNS TRIGGER
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;


/* Define the table. */
CREATE TABLE IF NOT EXISTS inventory (
    id SERIAL UNIQUE,
    img_id CHAR(8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    item_count INT NOT NULL,
    item_price REAL NOT NULL,
    item_brand VARCHAR(256) NOT NULL,
    item_name VARCHAR(512) NOT NULL,
    item_desc VARCHAR(4096) NOT NULL,
    PRIMARY KEY (id)
);


/* Attach a trigger for the "inventory_updated_at" function. */
CREATE TRIGGER inventory_updated_at_trigger
    BEFORE UPDATE ON inventory
    FOR EACH ROW
    EXECUTE PROCEDURE inventory_updated_at ();

