from flask import Flask, request, jsonify
from transformers import AutoTokenizer, AutoModel
from transformers import T5Tokenizer, T5EncoderModel
import torch
import threading

from transformers import AutoTokenizer, AutoModel
import torch

class EmbeddingService:
    def __init__(self, model_id):
        self.model = AutoModel.from_pretrained('jinaai/jina-embeddings-v2-base-en', trust_remote_code=True) # trust_remote_code is needed to use the encode method
        self.device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
        self.model.to(self.device)

    def get_embeddings(self, texts):
        outputs = self.model.encode(texts)
        return outputs.tolist()

# Initialize the Flask application
app = Flask(__name__)
lock = threading.Lock()
service = None

# Define the /embeddings endpoint
@app.route('/embeddings', methods=['POST'])
def get_embeddings():
    global service
    data = request.get_json()
    texts = data.get('texts', [])

    with lock:
        if service is None:
            service = EmbeddingService('jinaai/jina-embeddings-v2-base-en')
        embeddings = service.get_embeddings(texts)

    return jsonify({"vectors": embeddings})

# Run the Flask application
if __name__ == '__main__':
    app.run(debug=True)
