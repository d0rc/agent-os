from flask import Flask, request, jsonify
from vllm import LLM, SamplingParams
import time

app = Flask(__name__)

llm = LLM(model="TheBloke/dolphin-2.2.1-mistral-7B-AWQ", quantization="awq", dtype="half")

@app.route('/v1/completions', methods=['POST'])
def generate():
    data = request.get_json()
    prompts = data.get('prompts', data.get('prompt', None))
    
    # Ensure prompts is a list
    if isinstance(prompts, str):
        prompts = [prompts]
    elif not isinstance(prompts, list):
        return jsonify(error='Invalid or empty prompts list'), 400
    
    temperature = data.get('temperature', 0.8)
    top_p = data.get('top_p', 0.95)
    max_tokens = data.get('max_tokens', 4096)
    sampling_params = SamplingParams(temperature=temperature, top_p=top_p, max_tokens=max_tokens)

    
    outputs = llm.generate(prompts, sampling_params)
    formatted_outputs = [{'text': output.outputs[0].text} for output in outputs]
    # Assuming each character is a token for simplicity
    # You may need a more accurate way to count tokens based on your tokenization scheme
    prompt_tokens = sum(len(prompt) for prompt in prompts)
    completion_tokens = sum(len(output.outputs[0].text) for output in outputs)
    total_tokens = prompt_tokens + completion_tokens
    
    response = {
        "choices": formatted_outputs,
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "total_tokens": total_tokens
        }
    }
    
    return jsonify(response)

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)
