from datasets import Dataset
import json
import os
from collections import defaultdict

# Directory containing your JSON files
json_files_directory = '../voter-training-data/'

# Function to transform list of dicts into dict of lists
def transform_to_dict_of_lists(data_list):
    transformed_data = defaultdict(list)
    for item in data_list:
        for key, value in item.items():
            transformed_data[key].append(value)
    return dict(transformed_data)

# Read JSON files and aggregate them
data = []
for filename in os.listdir(json_files_directory):
    if filename.endswith('.json'):
        print("processing file ", len(data),": ", filename, "")
        with open(os.path.join(json_files_directory, filename), 'r') as file:
            data.append(json.load(file))

# Transform the data to the required format
transformed_data = transform_to_dict_of_lists(data)

# Convert to Hugging Face Dataset
dataset = Dataset.from_dict(transformed_data)

# Push dataset to your Hugging Face account (replace 'your_dataset_name' with your desired dataset name)
dataset.push_to_hub('onealeph0cc/voting-agents-dataset-3')

