db = db.getSiblingDB('data_privacy');

db.createUser({
    user: "user",
    pwd: "user!@#",
    roles: [
        { role: "readWrite", db: "data_privacy" }
    ]
});

const result = db.accounts.insertMany([
    {first_name:'Isaac',last_name:'Cheng',age:30,amount:1000.5,created_at:new Date(),updated_at:new Date()},
    {first_name:'John',last_name:'Doe',age:40,amount:2500.75,created_at:new Date(),updated_at:new Date()}
]);

const id1 = result.insertedIds["0"];
const id2 = result.insertedIds["1"];

db.transformed_accounts.insertMany([
    {account_id:id1,related_fields:'masked_email'},
    {account_id:id1,related_fields:'anonymized_phone'},
    {account_id:id2,related_fields:'encrypted_address'}
]);

db.transformed_accounts.createIndex({account_id:1});