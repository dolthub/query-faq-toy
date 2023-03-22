row_cnt = 200000
insert_batch_cnt = row_cnt / 20;
custom_plants="""insert into plants values
  (-1,'sunflower', 3,1),
  (-2,'carnation',0,1),
  (-3,'wildflower',4,3);
"""
custom_animals="""insert into animals values
  (-1,'goat',2,4),
  (-2,'cow',1,3),
  (-3,'chicken',0,1);
"""
if __name__=="__main__":
    with open("animals.sql", "w") as f:
        f.write("create table animals (id int primary key, name varchar(20), category int, elevation int, key (name), key (category), key (elevation))\n;")
        f.write(custom_animals)
        f.write("insert into animals values\n")
        for i in range(row_cnt):
            if i > 0 and i % insert_batch_cnt == 0:
                f.write(";\n")
                f.write("insert into animals values\n")
            elif i > 0:
                f.write(',\n')
            category = i % (row_cnt / 50)
            elevation = i % (row_cnt / 1000)
            f.write(f"  ({i},'name{i}',{category},{elevation})")
        f.write(";\n")
    with open("plants.sql", "w") as f:
        f.write("create table plants (id int primary key, name varchar(20), category int, elevation int, key (name), key (category), key (elevation))\n;")
        f.write(custom_plants)
        f.write("insert into plants values\n")
        for i in range(row_cnt):
            if i > 0 and i % insert_batch_cnt == 0:
                f.write(";\n")
                f.write("insert into plants values\n")
            elif i > 0:
                f.write(',\n')
            f.write(f"  ({i},'name{i}',{category},{elevation})")
            category = i % (row_cnt / 50)
            elevation = i % (row_cnt / 1000)
        f.write(";\n")

